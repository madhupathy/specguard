package scan

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/ghodss/yaml"
	canonicaljson "github.com/gibson042/canonicaljson-go"
	"github.com/jhump/protoreflect/desc/protoparse"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/specguard/specguard/internal/docindex"
	"github.com/specguard/specguard/internal/projectconfig"
)

// Runner executes normalization pipelines for SpecGuard scans.
type Runner struct{}

func collectProtoImportPaths(repoPath string, inputs []projectconfig.ProtobufInput) []string {
	seen := map[string]struct{}{}
	add := func(path string) {
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
	}

	add(repoPath)
	for _, input := range inputs {
		root := filepath.Clean(filepath.Join(repoPath, input.Root))
		add(root)
	}

	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

func canonicalizeOpenAPIFile(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if json.Valid(raw) {
		return canonicalize(raw)
	}
	jsonBytes, err := yaml.YAMLToJSON(raw)
	if err != nil {
		return nil, err
	}
	return canonicalize(jsonBytes)
}

func isKinOpenAPIBug(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "kin-openapi bug found")
}

func (r *Runner) captureMarkdown(repoPath, sourceRel, snapshotDir string) (manifestEntry, error) {
	absPath := filepath.Clean(filepath.Join(repoPath, sourceRel))
	contents, err := os.ReadFile(absPath)
	if err != nil {
		return manifestEntry{}, fmt.Errorf("read markdown %s: %w", sourceRel, err)
	}
	destPath := filepath.Join(snapshotDir, filepath.Base(sourceRel))
	if err := os.WriteFile(destPath, contents, 0o644); err != nil {
		return manifestEntry{}, fmt.Errorf("write markdown snapshot: %w", err)
	}

	return manifestEntry{
		Type:      "markdown",
		Source:    relPath(repoPath, absPath),
		Output:    relPath(repoPath, destPath),
		SizeBytes: int64(len(contents)),
		SHA256:    sha256Hex(contents),
	}, nil
}

func discoverOpenAPIInputs(repoPath string) ([]projectconfig.OpenAPIInput, error) {
	preferred := []string{
		"docs/apidocs.swagger.json",
		"docs/openapi.json",
		"docs/swagger.json",
	}
	for _, rel := range preferred {
		abs := filepath.Join(repoPath, rel)
		info, err := os.Stat(abs)
		if err == nil && !info.IsDir() {
			return []projectconfig.OpenAPIInput{{Path: rel}}, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("stat %s: %w", rel, err)
		}
	}

	docsDir := filepath.Join(repoPath, "docs")
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read docs dir: %w", err)
	}

	type candidate struct {
		path  string
		score int
	}
	var candidates []candidate
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".json") {
			continue
		}
		lower := strings.ToLower(name)
		score := 1
		if strings.Contains(lower, "swagger") {
			score += 4
		}
		if strings.Contains(lower, "openapi") {
			score += 4
		}
		if strings.Contains(lower, "api") {
			score += 2
		}
		candidates = append(candidates, candidate{path: filepath.Join("docs", name), score: score})
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].path > candidates[j].path
		}
		return candidates[i].score > candidates[j].score
	})

	return []projectconfig.OpenAPIInput{{Path: candidates[0].path}}, nil
}

func discoverMarkdownSpec(repoPath string) (string, error) {
	docsDir := filepath.Join(repoPath, "docs")
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read docs dir: %w", err)
	}

	type candidate struct {
		path  string
		score int
	}
	var candidates []candidate
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}
		lower := strings.ToLower(name)
		score := 1
		if strings.Contains(lower, "grpc") {
			score += 4
		}
		if strings.Contains(lower, "api") {
			score += 2
		}
		if strings.Contains(lower, "docs") {
			score++
		}
		candidates = append(candidates, candidate{path: filepath.Join("docs", name), score: score})
	}

	if len(candidates) == 0 {
		return "", nil
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].path > candidates[j].path
		}
		return candidates[i].score > candidates[j].score
	})

	return candidates[0].path, nil
}

// NewRunner constructs a Runner with default settings.
func NewRunner() *Runner {
	return &Runner{}
}

// Run executes the scan workflow for the provided repository/config.
func (r *Runner) Run(ctx context.Context, repoPath string, cfg projectconfig.Config, outDir string) error {
	snapshotDir := filepath.Join(outDir, "snapshot")
	if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
		return fmt.Errorf("create snapshot dir: %w", err)
	}

	mani := manifest{
		Version:     1,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Entries:     make([]manifestEntry, 0, 2),
	}

	openAPIInputs := cfg.Inputs.OpenAPI
	if len(openAPIInputs) == 0 {
		autoInputs, err := discoverOpenAPIInputs(repoPath)
		if err != nil {
			return err
		}
		openAPIInputs = autoInputs
	}

	if len(openAPIInputs) > 0 {
		if len(openAPIInputs) > 1 {
			return fmt.Errorf("multiple OpenAPI inputs not yet supported (got %d)", len(openAPIInputs))
		}
		entry, err := r.normalizeOpenAPI(ctx, repoPath, openAPIInputs[0], filepath.Join(snapshotDir, "openapi.normalized.json"))
		if err != nil {
			return err
		}
		mani.Entries = append(mani.Entries, entry)
	} else {
		mdPath, err := discoverMarkdownSpec(repoPath)
		if err != nil {
			return err
		}
		if mdPath != "" {
			entry, err := r.captureMarkdown(repoPath, mdPath, snapshotDir)
			if err != nil {
				return err
			}
			mani.Entries = append(mani.Entries, entry)
		}
	}

	if len(cfg.Inputs.Protobuf) > 0 {
		entry, err := r.normalizeProtobuf(ctx, repoPath, cfg.Inputs.Protobuf, filepath.Join(snapshotDir, "proto.normalized.json"))
		if err != nil {
			return err
		}
		mani.Entries = append(mani.Entries, entry)
	}

	var docSummary *docindex.Summary
	if cfg.Docs.SourceDir != "" {
		docIndexDir := filepath.Join(outDir, "doc_index")
		docSource := cfg.Docs.SourceDir
		if !filepath.IsAbs(docSource) {
			docSource = filepath.Join(repoPath, docSource)
		}
		summary, err := docindex.Build(ctx, docSource, docIndexDir)
		if err != nil {
			return fmt.Errorf("build doc index: %w", err)
		}
		docSummary = &summary
		entry := manifestEntry{
			Type:      "doc_index",
			Source:    relPath(repoPath, docSource),
			Output:    relPath(repoPath, docIndexDir),
			SizeBytes: summary.TotalBytes,
			SHA256:    summary.Digest,
			Count:     summary.TotalChunks,
		}
		mani.Entries = append(mani.Entries, entry)
	}

	manifestPath := filepath.Join(snapshotDir, "manifest.json")
	if err := writeCanonicalJSON(manifestPath, mani); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	if err := writeSpecSnapshot(outDir, manifestPath, mani); err != nil {
		return fmt.Errorf("write spec snapshot: %w", err)
	}

	if err := writeKnowledgeModel(outDir, manifestPath, mani, docSummary); err != nil {
		return fmt.Errorf("write knowledge model: %w", err)
	}

	if len(mani.Entries) == 0 {
		return fmt.Errorf("no inputs discovered; add config.inputs or ensure docs/json specs exist")
	}

	return nil
}

func writeSpecSnapshot(outDir, manifestPath string, mani manifest) error {
	snapshot := struct {
		GeneratedAt  string          `json:"generated_at"`
		ManifestPath string          `json:"manifest_path"`
		EntryCount   int             `json:"entry_count"`
		Entries      []manifestEntry `json:"entries"`
	}{
		GeneratedAt:  mani.GeneratedAt,
		ManifestPath: manifestPath,
		EntryCount:   len(mani.Entries),
		Entries:      mani.Entries,
	}

	path := filepath.Join(outDir, "spec_snapshot.json")
	return writeCanonicalJSON(path, snapshot)
}

func writeKnowledgeModel(outDir, manifestPath string, mani manifest, docSummary *docindex.Summary) error {
	type kmSpec struct {
		Type      string `json:"type"`
		Source    string `json:"source"`
		Output    string `json:"output"`
		SHA256    string `json:"sha256"`
		SizeBytes int64  `json:"size_bytes"`
		Count     int    `json:"count,omitempty"`
	}

	type kmDocIndex struct {
		SourceDir    string `json:"source_dir"`
		ManifestPath string `json:"manifest_path"`
		ChunksPath   string `json:"chunks_path"`
		Embeddings   string `json:"embeddings_path"`
		TotalChunks  int    `json:"total_chunks"`
		TotalBytes   int64  `json:"total_bytes"`
		Digest       string `json:"sha256"`
		GeneratedAt  string `json:"generated_at"`
	}

	model := struct {
		GeneratedAt  string      `json:"generated_at"`
		ManifestPath string      `json:"manifest_path"`
		Specs        []kmSpec    `json:"specs"`
		DocIndex     *kmDocIndex `json:"doc_index,omitempty"`
	}{
		GeneratedAt:  mani.GeneratedAt,
		ManifestPath: manifestPath,
		Specs:        make([]kmSpec, 0, len(mani.Entries)),
	}

	for _, entry := range mani.Entries {
		if entry.Type == "doc_index" {
			continue
		}
		model.Specs = append(model.Specs, kmSpec{
			Type:      entry.Type,
			Source:    entry.Source,
			Output:    entry.Output,
			SHA256:    entry.SHA256,
			SizeBytes: entry.SizeBytes,
			Count:     entry.Count,
		})
	}

	if docSummary != nil {
		model.DocIndex = &kmDocIndex{
			SourceDir:    docSummary.SourceDir,
			ManifestPath: docSummary.ManifestPath,
			ChunksPath:   docSummary.ChunksPath,
			Embeddings:   docSummary.EmbeddingsPath,
			TotalChunks:  docSummary.TotalChunks,
			TotalBytes:   docSummary.TotalBytes,
			Digest:       docSummary.Digest,
			GeneratedAt:  docSummary.GeneratedAt,
		}
	}

	path := filepath.Join(outDir, "knowledge_model.json")
	return writeCanonicalJSON(path, model)
}

func (r *Runner) normalizeOpenAPI(ctx context.Context, repoPath string, input projectconfig.OpenAPIInput, outputPath string) (manifestEntry, error) {
	absPath := filepath.Clean(filepath.Join(repoPath, input.Path))
	var canonical []byte

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.Context = ctx

	doc, err := loader.LoadFromFile(absPath)
	switch {
	case err == nil:
		if validateErr := doc.Validate(ctx); validateErr != nil && !isKinOpenAPIBug(validateErr) {
			return manifestEntry{}, fmt.Errorf("validate OpenAPI spec %s: %w", input.Path, validateErr)
		}
		rawJSON, err := doc.MarshalJSON()
		if err != nil {
			return manifestEntry{}, fmt.Errorf("marshal OpenAPI spec %s: %w", input.Path, err)
		}
		canonical, err = canonicalize(rawJSON)
		if err != nil {
			return manifestEntry{}, fmt.Errorf("canonicalize OpenAPI spec %s: %w", input.Path, err)
		}
	default:
		if !isKinOpenAPIBug(err) {
			return manifestEntry{}, fmt.Errorf("load OpenAPI spec %s: %w", input.Path, err)
		}
		canonical, err = canonicalizeOpenAPIFile(absPath)
		if err != nil {
			return manifestEntry{}, fmt.Errorf("fallback canonicalize OpenAPI %s: %w", input.Path, err)
		}
	}

	if err := os.WriteFile(outputPath, canonical, 0o644); err != nil {
		return manifestEntry{}, fmt.Errorf("write OpenAPI snapshot: %w", err)
	}

	return manifestEntry{
		Type:      "openapi",
		Source:    relPath(repoPath, absPath),
		Output:    relPath(repoPath, outputPath),
		SizeBytes: int64(len(canonical)),
		SHA256:    sha256Hex(canonical),
	}, nil
}

func (r *Runner) normalizeProtobuf(ctx context.Context, repoPath string, inputs []projectconfig.ProtobufInput, outputPath string) (manifestEntry, error) {
	files, err := collectProtoFiles(repoPath, inputs)
	if err != nil {
		return manifestEntry{}, err
	}

	importPaths := collectProtoImportPaths(repoPath, inputs)
	parser := protoparse.Parser{
		ImportPaths:           importPaths,
		IncludeSourceCodeInfo: true,
	}

	// protoparse expects OS-specific separators
	protoFiles := make([]string, len(files))
	for i, f := range files {
		protoFiles[i] = filepath.FromSlash(f)
	}

	var descriptors []*descriptorpb.FileDescriptorProto
	if len(protoFiles) > 0 {
		fds, err := parser.ParseFiles(protoFiles...)
		if err != nil {
			return manifestEntry{}, fmt.Errorf("parse proto files: %w", err)
		}
		for _, fd := range fds {
			descriptors = append(descriptors, fd.AsFileDescriptorProto())
		}
	}

	set := &descriptorpb.FileDescriptorSet{File: descriptors}
	protoJSON, err := protojson.MarshalOptions{Multiline: false, EmitUnpopulated: true}.Marshal(set)
	if err != nil {
		return manifestEntry{}, fmt.Errorf("marshal descriptor set: %w", err)
	}

	canonical, err := canonicalize(protoJSON)
	if err != nil {
		return manifestEntry{}, fmt.Errorf("canonicalize descriptor json: %w", err)
	}

	if err := os.WriteFile(outputPath, canonical, 0o644); err != nil {
		return manifestEntry{}, fmt.Errorf("write proto snapshot: %w", err)
	}

	source := strings.Join(files, ",")
	if source == "" {
		source = "(none)"
	}

	return manifestEntry{
		Type:      "protobuf",
		Source:    source,
		Output:    relPath(repoPath, outputPath),
		SizeBytes: int64(len(canonical)),
		SHA256:    sha256Hex(canonical),
		Count:     len(files),
	}, nil
}

func collectProtoFiles(repoPath string, inputs []projectconfig.ProtobufInput) ([]string, error) {
	unique := map[string]struct{}{}
	files := []string{}

	for _, input := range inputs {
		root := filepath.Clean(filepath.Join(repoPath, input.Root))
		patterns := input.Include
		if len(patterns) == 0 {
			patterns = []string{"**/*.proto"}
		}

		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			relToRoot, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			relToRoot = filepath.ToSlash(relToRoot)
			for _, pattern := range patterns {
				match, matchErr := doublestar.Match(pattern, relToRoot)
				if matchErr != nil {
					return matchErr
				}
				if match {
					key := fmt.Sprintf("%s::%s", input.Root, relToRoot)
					if _, exists := unique[key]; !exists {
						unique[key] = struct{}{}
						files = append(files, relToRoot)
					}
					break
				}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk proto root %s: %w", input.Root, err)
		}
	}

	sort.Strings(files)
	return files, nil
}

func canonicalize(raw []byte) ([]byte, error) {
	var generic interface{}
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, err
	}
	return canonicaljson.Marshal(generic)
}

func writeCanonicalJSON(path string, v interface{}) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	canonical, err := canonicalize(raw)
	if err != nil {
		return err
	}
	return os.WriteFile(path, canonical, 0o644)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}

func relPath(root, target string) string {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return target
	}
	return filepath.ToSlash(rel)
}

type manifest struct {
	Version     int             `json:"version"`
	GeneratedAt string          `json:"generated_at"`
	Entries     []manifestEntry `json:"entries"`
}

type manifestEntry struct {
	Type      string `json:"type"`
	Source    string `json:"source"`
	Output    string `json:"output"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
	Count     int    `json:"count,omitempty"`
}
