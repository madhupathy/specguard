// Package enrich generates an enriched OpenAPI spec by matching doc chunks
// to endpoints and injecting descriptions and usage examples.
package enrich

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type chunkRecord struct {
	ID         string `json:"id"`
	Source     string `json:"source"`
	ChunkIndex int    `json:"chunk_index"`
	Content    string `json:"content"`
}

// EnrichResult tracks what was enriched.
type EnrichResult struct {
	TotalEndpoints    int `json:"total_endpoints"`
	EnrichedEndpoints int `json:"enriched_endpoints"`
	ChunksMatched     int `json:"chunks_matched"`
}

// EnrichSpec reads a normalized OpenAPI spec and doc chunks, matches chunks
// to endpoints by keyword overlap, and writes an enriched spec with
// x-specguard-docs annotations plus a standalone examples file.
func EnrichSpec(specPath, chunksPath, outDir string) (EnrichResult, error) {
	result := EnrichResult{}

	raw, err := os.ReadFile(specPath)
	if err != nil {
		return result, fmt.Errorf("read spec: %w", err)
	}
	var spec map[string]interface{}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return result, fmt.Errorf("parse spec: %w", err)
	}

	chunks, err := loadChunks(chunksPath)
	if err != nil {
		return result, fmt.Errorf("load chunks: %w", err)
	}

	paths, _ := spec["paths"].(map[string]interface{})
	if paths == nil {
		return result, fmt.Errorf("no paths in spec")
	}

	methods := []string{"get", "post", "put", "delete", "patch"}
	examples := map[string]interface{}{}

	for path, pathVal := range paths {
		pathObj, ok := pathVal.(map[string]interface{})
		if !ok {
			continue
		}
		for _, method := range methods {
			opVal, ok := pathObj[method]
			if !ok {
				continue
			}
			op, ok := opVal.(map[string]interface{})
			if !ok {
				continue
			}
			result.TotalEndpoints++

			endpoint := fmt.Sprintf("%s %s", strings.ToUpper(method), path)
			matched := matchChunks(endpoint, path, op, chunks)
			if len(matched) == 0 {
				continue
			}

			result.EnrichedEndpoints++
			result.ChunksMatched += len(matched)

			// Build enrichment annotation
			var docSnippets []interface{}
			for _, chunk := range matched {
				snippet := truncate(chunk.Content, 500)
				docSnippets = append(docSnippets, map[string]interface{}{
					"source":      chunk.Source,
					"chunk_index": chunk.ChunkIndex,
					"excerpt":     snippet,
				})
			}
			op["x-specguard-docs"] = docSnippets

			// If no description, add one from best chunk
			if _, hasDesc := op["description"]; !hasDesc {
				best := matched[0]
				desc := extractDescription(best.Content, path)
				if desc != "" {
					op["description"] = desc
				}
			}

			// Build example entry
			exampleEntry := map[string]interface{}{
				"endpoint":    endpoint,
				"doc_sources": []string{},
			}
			sources := map[string]bool{}
			for _, m := range matched {
				if !sources[m.Source] {
					sources[m.Source] = true
					exampleEntry["doc_sources"] = append(exampleEntry["doc_sources"].([]string), m.Source)
				}
			}
			// Extract any JSON-like examples from chunks
			for _, m := range matched {
				if ex := extractJSONExample(m.Content); ex != "" {
					exampleEntry["example_snippet"] = ex
					break
				}
			}
			examples[endpoint] = exampleEntry
		}
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return result, fmt.Errorf("create output dir: %w", err)
	}

	// Write enrichment summary markdown only
	md := renderSummary(result, examples)
	if err := os.WriteFile(filepath.Join(outDir, "enrichment_summary.md"), []byte(md), 0o644); err != nil {
		return result, err
	}

	return result, nil
}

func loadChunks(path string) ([]chunkRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var chunks []chunkRecord
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		var c chunkRecord
		if err := json.Unmarshal(scanner.Bytes(), &c); err != nil {
			continue
		}
		chunks = append(chunks, c)
	}
	return chunks, scanner.Err()
}

// matchChunks finds doc chunks relevant to a given endpoint.
func matchChunks(endpoint, path string, op map[string]interface{}, chunks []chunkRecord) []chunkRecord {
	// Build search terms from path segments and operation metadata
	terms := extractSearchTerms(path, op)
	if len(terms) == 0 {
		return nil
	}

	type scored struct {
		chunk chunkRecord
		score int
	}
	var matches []scored

	for _, chunk := range chunks {
		lower := strings.ToLower(chunk.Content)
		score := 0
		for _, term := range terms {
			if len(term) >= 3 && strings.Contains(lower, strings.ToLower(term)) {
				score++
			}
		}
		// Also check if the path itself appears in the chunk
		if strings.Contains(lower, strings.ToLower(path)) {
			score += 5
		}
		if score >= 2 {
			matches = append(matches, scored{chunk: chunk, score: score})
		}
	}

	// Sort by score descending, take top 3
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].score > matches[i].score {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	limit := 3
	if len(matches) < limit {
		limit = len(matches)
	}
	result := make([]chunkRecord, limit)
	for i := 0; i < limit; i++ {
		result[i] = matches[i].chunk
	}
	return result
}

func extractSearchTerms(path string, op map[string]interface{}) []string {
	var terms []string

	// Path segments (skip version prefix and path params)
	segments := strings.Split(path, "/")
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" || strings.HasPrefix(seg, "v") && len(seg) <= 3 {
			continue
		}
		if strings.HasPrefix(seg, "{") {
			continue
		}
		// Split camelCase and kebab-case
		parts := strings.FieldsFunc(seg, func(r rune) bool {
			return r == '-' || r == '_'
		})
		terms = append(terms, parts...)
		terms = append(terms, seg)
	}

	// OperationId
	if opID, ok := op["operationId"].(string); ok {
		terms = append(terms, opID)
		// Split camelCase
		words := splitCamelCase(opID)
		terms = append(terms, words...)
	}

	// Summary
	if summary, ok := op["summary"].(string); ok {
		words := strings.Fields(summary)
		for _, w := range words {
			if len(w) >= 4 {
				terms = append(terms, w)
			}
		}
	}

	// Tags
	if tags, ok := op["tags"].([]interface{}); ok {
		for _, t := range tags {
			if s, ok := t.(string); ok {
				terms = append(terms, s)
			}
		}
	}

	return dedupe(terms)
}

func splitCamelCase(s string) []string {
	var words []string
	var current strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

func dedupe(items []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, item := range items {
		lower := strings.ToLower(item)
		if !seen[lower] && len(item) >= 3 {
			seen[lower] = true
			result = append(result, item)
		}
	}
	return result
}

func extractDescription(content, path string) string {
	// Try to find a sentence that mentions key path terms
	segments := strings.Split(path, "/")
	var keyTerms []string
	for _, seg := range segments {
		if seg != "" && !strings.HasPrefix(seg, "{") && !strings.HasPrefix(seg, "v") {
			keyTerms = append(keyTerms, strings.ToLower(seg))
		}
	}

	sentences := strings.Split(content, ".")
	for _, sentence := range sentences {
		lower := strings.ToLower(sentence)
		matches := 0
		for _, term := range keyTerms {
			if strings.Contains(lower, term) {
				matches++
			}
		}
		if matches >= 1 && len(sentence) > 20 && len(sentence) < 300 {
			return strings.TrimSpace(sentence) + "."
		}
	}
	return truncate(content, 200)
}

func extractJSONExample(content string) string {
	// Look for JSON-like blocks in the content
	start := strings.Index(content, "{")
	if start < 0 {
		return ""
	}
	depth := 0
	end := -1
	for i := start; i < len(content); i++ {
		switch content[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
		if end > 0 {
			break
		}
	}
	if end <= start || end-start < 10 || end-start > 1000 {
		return ""
	}
	candidate := content[start:end]
	// Validate it's actually JSON
	var js json.RawMessage
	if json.Unmarshal([]byte(candidate), &js) == nil {
		return candidate
	}
	return ""
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func renderSummary(result EnrichResult, examples map[string]interface{}) string {
	var b strings.Builder
	b.WriteString("# Enriched Swagger Summary\n\n")
	b.WriteString(fmt.Sprintf("- Total endpoints: **%d**\n", result.TotalEndpoints))
	b.WriteString(fmt.Sprintf("- Enriched with docs: **%d**\n", result.EnrichedEndpoints))
	b.WriteString(fmt.Sprintf("- Doc chunks matched: **%d**\n", result.ChunksMatched))
	coverage := float64(0)
	if result.TotalEndpoints > 0 {
		coverage = float64(result.EnrichedEndpoints) / float64(result.TotalEndpoints) * 100
	}
	b.WriteString(fmt.Sprintf("- Coverage: **%.1f%%**\n\n", coverage))

	if len(examples) > 0 {
		b.WriteString("## Enriched Endpoints\n\n")
		b.WriteString("| Endpoint | Doc Sources |\n")
		b.WriteString("| --- | --- |\n")
		for endpoint, val := range examples {
			entry, _ := val.(map[string]interface{})
			sources, _ := entry["doc_sources"].([]string)
			b.WriteString(fmt.Sprintf("| `%s` | %s |\n", endpoint, strings.Join(sources, ", ")))
		}
	}

	return b.String()
}
