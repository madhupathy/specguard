package docindex

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	pdf "github.com/ledongthuc/pdf"
)

// Summary captures the results of document ingestion for later knowledge-model wiring.
type Summary struct {
	SourceDir       string            `json:"source_dir"`
	Documents       []DocumentSummary `json:"documents"`
	TotalChunks     int               `json:"total_chunks"`
	TotalEmbeddings int               `json:"total_embeddings"`
	TotalBytes      int64             `json:"total_bytes"`
	ChunksPath      string            `json:"chunks_path"`
	EmbeddingsPath  string            `json:"embeddings_path"`
	ManifestPath    string            `json:"manifest_path"`
	Digest          string            `json:"sha256"`
	GeneratedAt     string            `json:"generated_at"`
}

// DocumentSummary captures per-document metadata inside a docs directory.
type DocumentSummary struct {
	Path       string `json:"path"`
	Type       string `json:"type"`
	SizeBytes  int64  `json:"size_bytes"`
	ChunkCount int    `json:"chunk_count"`
}

// chunkRecord is written to chunks.jsonl.
type chunkRecord struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"`
	Chunk     int       `json:"chunk_index"`
	Content   string    `json:"content"`
	Embedding []float32 `json:"embedding"`
}

// embeddingRecord is written to embeddings.jsonl.
type embeddingRecord struct {
	ID     string    `json:"id"`
	Vector []float32 `json:"vector"`
}

// Build walks docsDir, extracts plaintext, chunks it, and writes chunk + embedding jsonl files under outDir.
func Build(ctx context.Context, docsDir, outDir string) (Summary, error) {
	var summary Summary
	if docsDir == "" {
		return summary, fmt.Errorf("docs directory is empty")
	}

	info, err := os.Stat(docsDir)
	if err != nil {
		return summary, fmt.Errorf("stat docs dir: %w", err)
	}
	if !info.IsDir() {
		return summary, fmt.Errorf("docs dir is not a directory: %s", docsDir)
	}

	if err := os.RemoveAll(outDir); err != nil {
		return summary, fmt.Errorf("clear doc_index dir: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return summary, fmt.Errorf("create doc_index dir: %w", err)
	}

	chunksPath := filepath.Join(outDir, "chunks.jsonl")
	embeddingsPath := filepath.Join(outDir, "embeddings.jsonl")

	chunksFile, err := os.Create(chunksPath)
	if err != nil {
		return summary, fmt.Errorf("create chunks file: %w", err)
	}
	defer chunksFile.Close()

	embeddingsFile, err := os.Create(embeddingsPath)
	if err != nil {
		return summary, fmt.Errorf("create embeddings file: %w", err)
	}
	defer embeddingsFile.Close()

	chunksWriter := bufio.NewWriter(chunksFile)
	embeddingsWriter := bufio.NewWriter(embeddingsFile)
	defer chunksWriter.Flush()
	defer embeddingsWriter.Flush()

	summary = Summary{
		SourceDir:      docsDir,
		Documents:      []DocumentSummary{},
		ChunksPath:     chunksPath,
		EmbeddingsPath: embeddingsPath,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}

	err = filepath.WalkDir(docsDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !supportedDocExtension(ext) {
			return nil
		}

		text, err := extractText(path, ext)
		if err != nil {
			return fmt.Errorf("extract text from %s: %w", path, err)
		}
		text = strings.TrimSpace(text)
		if text == "" {
			return nil
		}

		chunks := chunkText(text, 200)
		if len(chunks) == 0 {
			return nil
		}

		stat, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("stat doc %s: %w", path, err)
		}

		relPath, err := filepath.Rel(docsDir, path)
		if err != nil {
			relPath = filepath.Base(path)
		}
		relPath = filepath.ToSlash(relPath)

		docSummary := DocumentSummary{
			Path:      relPath,
			Type:      strings.TrimPrefix(ext, "."),
			SizeBytes: stat.Size(),
		}

		for idx, chunk := range chunks {
			chunkID := fmt.Sprintf("%s#chunk-%d", relPath, idx)
			record := chunkRecord{
				ID:        chunkID,
				Source:    relPath,
				Chunk:     idx,
				Content:   chunk,
				Embedding: pseudoEmbedding(chunk),
			}
			if err := writeJSONLine(chunksWriter, record); err != nil {
				return err
			}
			embedRec := embeddingRecord{ID: chunkID, Vector: record.Embedding}
			if err := writeJSONLine(embeddingsWriter, embedRec); err != nil {
				return err
			}
		}

		docSummary.ChunkCount = len(chunks)
		summary.TotalChunks += docSummary.ChunkCount
		summary.TotalEmbeddings += docSummary.ChunkCount
		summary.Documents = append(summary.Documents, docSummary)
		return nil
	})
	if err != nil {
		return summary, err
	}

	manifestPath := filepath.Join(outDir, "manifest.json")
	summary.ManifestPath = manifestPath
	if err := writeJSONFile(manifestPath, summary); err != nil {
		return summary, err
	}

	totalBytes, digest, err := digestFiles(manifestPath, chunksPath, embeddingsPath)
	if err != nil {
		return summary, err
	}
	summary.TotalBytes = totalBytes
	summary.Digest = digest

	if err := writeJSONFile(manifestPath, summary); err != nil {
		return summary, err
	}

	return summary, nil
}

func supportedDocExtension(ext string) bool {
	switch ext {
	case ".md", ".markdown", ".txt", ".pdf":
		return true
	default:
		return false
	}
}

func extractText(path, ext string) (string, error) {
	switch ext {
	case ".md", ".markdown", ".txt":
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	case ".pdf":
		f, r, err := pdf.Open(path)
		if err != nil {
			return "", err
		}
		defer f.Close()
		var builder strings.Builder
		totalPage := r.NumPage()
		for i := 1; i <= totalPage; i++ {
			page := r.Page(i)
			if page.V.IsNull() {
				continue
			}
			plain, err := page.GetPlainText(nil)
			if err != nil {
				return "", err
			}
			builder.WriteString(plain)
			builder.WriteString("\n")
		}
		return builder.String(), nil
	default:
		return "", fmt.Errorf("unsupported extension %s", ext)
	}
}

func chunkText(text string, maxWords int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	chunks := []string{}
	for start := 0; start < len(words); start += maxWords {
		end := start + maxWords
		if end > len(words) {
			end = len(words)
		}
		chunk := strings.Join(words[start:end], " ")
		chunks = append(chunks, chunk)
	}
	return chunks
}

func pseudoEmbedding(text string) []float32 {
	hash := sha256.Sum256([]byte(text))
	vec := make([]float32, 8)
	for i, b := range hash {
		vec[i%8] += float32(b) / 255.0
	}
	return vec
}

func writeJSONLine(w *bufio.Writer, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	if err := w.WriteByte('\n'); err != nil {
		return err
	}
	return nil
}

func writeJSONFile(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal doc manifest: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

func digestFiles(paths ...string) (int64, string, error) {
	h := sha256.New()
	var total int64
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return 0, "", fmt.Errorf("open %s: %w", path, err)
		}
		n, err := io.Copy(h, f)
		f.Close()
		if err != nil {
			return 0, "", fmt.Errorf("read %s: %w", path, err)
		}
		total += n
	}
	return total, fmt.Sprintf("%x", h.Sum(nil)), nil
}
