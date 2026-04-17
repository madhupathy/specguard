package docconsistency

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Issue represents a single doc-vs-spec consistency finding.
type Issue struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	Path        string `json:"path"`
	Description string `json:"description"`
	Remediation string `json:"remediation"`
}

// Report captures the full doc consistency analysis.
type Report struct {
	GeneratedAt string  `json:"generated_at"`
	TotalIssues int     `json:"total_issues"`
	Issues      []Issue `json:"issues"`
}

// chunkRecord mirrors the docindex chunk schema.
type chunkRecord struct {
	ID      string `json:"id"`
	Source  string `json:"source"`
	Chunk   int    `json:"chunk_index"`
	Content string `json:"content"`
}

// Analyze compares doc chunks against spec endpoints/schemas to find inconsistencies.
// specPath is the normalized OpenAPI JSON; chunksPath is the doc_index/chunks.jsonl.
func Analyze(specPath, chunksPath string) (Report, error) {
	report := Report{Issues: []Issue{}}

	spec, err := loadSpec(specPath)
	if err != nil {
		return report, err
	}

	chunks, err := loadChunks(chunksPath)
	if err != nil {
		return report, err
	}

	endpoints := extractEndpoints(spec)
	schemas := extractSchemaNames(spec)
	docText := joinChunks(chunks)
	docLower := strings.ToLower(docText)

	for _, ep := range endpoints {
		if !mentionedInDocs(ep, docLower) {
			report.Issues = append(report.Issues, Issue{
				Type:        "undocumented_endpoint",
				Severity:    "medium",
				Path:        ep,
				Description: fmt.Sprintf("Endpoint %s not mentioned in documentation", ep),
				Remediation: "Add documentation covering this endpoint's purpose and usage.",
			})
		}
	}

	for _, schema := range schemas {
		if !strings.Contains(docLower, strings.ToLower(schema)) {
			report.Issues = append(report.Issues, Issue{
				Type:        "undocumented_schema",
				Severity:    "low",
				Path:        fmt.Sprintf("components.schemas.%s", schema),
				Description: fmt.Sprintf("Schema %s not mentioned in documentation", schema),
				Remediation: "Add documentation describing this data model.",
			})
		}
	}

	docEndpoints := extractDocEndpoints(docText)
	for _, docEP := range docEndpoints {
		found := false
		for _, specEP := range endpoints {
			if strings.Contains(strings.ToLower(specEP), strings.ToLower(docEP)) {
				found = true
				break
			}
		}
		if !found {
			report.Issues = append(report.Issues, Issue{
				Type:        "doc_only_endpoint",
				Severity:    "medium",
				Path:        docEP,
				Description: fmt.Sprintf("Documentation references endpoint %s not found in spec", docEP),
				Remediation: "Either add the endpoint to the spec or remove the stale documentation.",
			})
		}
	}

	report.TotalIssues = len(report.Issues)
	return report, nil
}

// WriteReport writes the doc consistency report as Markdown.
func WriteReport(report Report, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create reports dir: %w", err)
	}

	md := renderMarkdown(report)
	return os.WriteFile(filepath.Join(outDir, "doc_consistency.md"), []byte(md), 0o644)
}

func renderMarkdown(report Report) string {
	var b strings.Builder
	b.WriteString("# Documentation Consistency Report\n\n")
	b.WriteString(fmt.Sprintf("Issues found: **%d**\n\n", report.TotalIssues))

	if len(report.Issues) == 0 {
		b.WriteString("No consistency issues detected.\n")
		return b.String()
	}

	b.WriteString("| # | Type | Severity | Path | Description | Remediation |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for i, issue := range report.Issues {
		b.WriteString(fmt.Sprintf("| %d | %s | %s | `%s` | %s | %s |\n",
			i+1, issue.Type, issue.Severity, issue.Path, issue.Description, issue.Remediation))
	}

	return b.String()
}

func loadSpec(path string) (map[string]interface{}, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read spec: %w", err)
	}
	var spec map[string]interface{}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, fmt.Errorf("parse spec: %w", err)
	}
	return spec, nil
}

func loadChunks(path string) ([]chunkRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open chunks: %w", err)
	}
	defer f.Close()

	var chunks []chunkRecord
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		var rec chunkRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		chunks = append(chunks, rec)
	}
	return chunks, scanner.Err()
}

func extractEndpoints(spec map[string]interface{}) []string {
	var endpoints []string
	paths := getMap(spec, "paths")
	methods := []string{"get", "post", "put", "delete", "patch", "head", "options"}
	for path, pathVal := range paths {
		pathInfo := asMap(pathVal)
		if pathInfo == nil {
			continue
		}
		for _, method := range methods {
			if _, ok := pathInfo[method]; ok {
				endpoints = append(endpoints, fmt.Sprintf("%s %s", strings.ToUpper(method), path))
			}
		}
	}
	return endpoints
}

func extractSchemaNames(spec map[string]interface{}) []string {
	var names []string
	schemas := getNestedMap(spec, "components", "schemas")
	for name := range schemas {
		names = append(names, name)
	}
	return names
}

func joinChunks(chunks []chunkRecord) string {
	var parts []string
	for _, c := range chunks {
		parts = append(parts, c.Content)
	}
	return strings.Join(parts, " ")
}

func mentionedInDocs(endpoint, docLower string) bool {
	parts := strings.SplitN(endpoint, " ", 2)
	if len(parts) != 2 {
		return false
	}
	path := strings.ToLower(parts[1])
	pathSegments := strings.Split(strings.Trim(path, "/"), "/")

	significantParts := []string{}
	for _, seg := range pathSegments {
		if !strings.HasPrefix(seg, "{") && seg != "api" && seg != "v1" && seg != "v2" && seg != "v3" {
			significantParts = append(significantParts, seg)
		}
	}

	if len(significantParts) == 0 {
		return true
	}

	for _, part := range significantParts {
		if strings.Contains(docLower, part) {
			return true
		}
	}
	return false
}

func extractDocEndpoints(text string) []string {
	var endpoints []string
	words := strings.Fields(text)
	methods := map[string]bool{"GET": true, "POST": true, "PUT": true, "DELETE": true, "PATCH": true}

	for i := 0; i < len(words)-1; i++ {
		upper := strings.ToUpper(strings.Trim(words[i], "`,.:;"))
		if methods[upper] {
			next := strings.Trim(words[i+1], "`,.:;")
			if strings.HasPrefix(next, "/") {
				endpoints = append(endpoints, upper+" "+next)
			}
		}
	}
	return endpoints
}

func asMap(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return nil
}

func getMap(data map[string]interface{}, key string) map[string]interface{} {
	if v, ok := data[key]; ok {
		if m := asMap(v); m != nil {
			return m
		}
	}
	return map[string]interface{}{}
}

func getNestedMap(data map[string]interface{}, keys ...string) map[string]interface{} {
	current := data
	for _, key := range keys {
		next := asMap(current[key])
		if next == nil {
			return map[string]interface{}{}
		}
		current = next
	}
	return current
}
