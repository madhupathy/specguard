package rag

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/specguard/specguard/internal/vectordb"
)

// EndpointDocLink maps a spec endpoint to its most relevant doc chunks.
type EndpointDocLink struct {
	Endpoint string        `json:"endpoint"`
	Links    []DocLink     `json:"links"`
	Coverage string        `json:"coverage"` // "documented", "partial", "undocumented"
}

// DocLink is a single chunk reference with relevance score.
type DocLink struct {
	ChunkID  string  `json:"chunk_id"`
	Source   string  `json:"source"`
	Score    float64 `json:"score"`
	Snippet  string  `json:"snippet"`
}

// KnowledgeEnrichment is the output of the RAG retrieval pass.
type KnowledgeEnrichment struct {
	GeneratedAt    string            `json:"generated_at"`
	TotalEndpoints int               `json:"total_endpoints"`
	Documented     int               `json:"documented"`
	Partial        int               `json:"partial"`
	Undocumented   int               `json:"undocumented"`
	Links          []EndpointDocLink `json:"endpoint_doc_links"`
}

// Retriever performs RAG-style retrieval linking spec endpoints to doc chunks.
type Retriever struct {
	store    *vectordb.Store
	embedder vectordb.Embedder
	topK     int
}

// NewRetriever creates a Retriever backed by the given store and embedder.
func NewRetriever(store *vectordb.Store, embedder vectordb.Embedder, topK int) *Retriever {
	if topK <= 0 {
		topK = 3
	}
	return &Retriever{store: store, embedder: embedder, topK: topK}
}

// LinkEndpoints takes a list of endpoint strings (e.g. "GET /users") and finds
// the most relevant doc chunks for each one via embedding similarity search.
func (r *Retriever) LinkEndpoints(endpoints []string) (KnowledgeEnrichment, error) {
	enrichment := KnowledgeEnrichment{
		Links: []EndpointDocLink{},
	}

	for _, ep := range endpoints {
		queryVec, err := r.embedder.Embed(ep)
		if err != nil {
			return enrichment, fmt.Errorf("embed endpoint %q: %w", ep, err)
		}

		results, err := r.store.Search(queryVec, r.topK)
		if err != nil {
			return enrichment, fmt.Errorf("search for %q: %w", ep, err)
		}

		link := EndpointDocLink{
			Endpoint: ep,
			Links:    []DocLink{},
		}

		for _, res := range results {
			if res.Score < 0.1 {
				continue
			}
			snippet := res.Content
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
			link.Links = append(link.Links, DocLink{
				ChunkID: res.ID,
				Source:  res.Source,
				Score:   res.Score,
				Snippet: snippet,
			})
		}

		switch {
		case len(link.Links) == 0:
			link.Coverage = "undocumented"
			enrichment.Undocumented++
		case link.Links[0].Score >= 0.5:
			link.Coverage = "documented"
			enrichment.Documented++
		default:
			link.Coverage = "partial"
			enrichment.Partial++
		}

		enrichment.Links = append(enrichment.Links, link)
	}

	enrichment.TotalEndpoints = len(endpoints)
	return enrichment, nil
}

// IngestChunksToStore reads a chunks.jsonl file and loads all chunks into the vector store.
func IngestChunksToStore(chunksPath string, store *vectordb.Store, embedder vectordb.Embedder) (int, error) {
	raw, err := os.ReadFile(chunksPath)
	if err != nil {
		return 0, fmt.Errorf("read chunks: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var records []vectordb.Record

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var chunk struct {
			ID       string `json:"id"`
			Source   string `json:"source"`
			ChunkIdx int    `json:"chunk_index"`
			Content  string `json:"content"`
		}
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue
		}

		vec, err := embedder.Embed(chunk.Content)
		if err != nil {
			continue
		}

		records = append(records, vectordb.Record{
			ID:        chunk.ID,
			Source:    chunk.Source,
			ChunkIdx:  chunk.ChunkIdx,
			Content:   chunk.Content,
			Embedding: vec,
		})
	}

	if len(records) > 0 {
		if err := store.UpsertBatch(records); err != nil {
			return 0, fmt.Errorf("upsert batch: %w", err)
		}
	}

	return len(records), nil
}

// WriteEnrichment writes the enrichment data as JSON and Markdown.
func WriteEnrichment(enrichment KnowledgeEnrichment, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	jsonData, err := json.MarshalIndent(enrichment, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal enrichment: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "doc_links.json"), jsonData, 0o644); err != nil {
		return err
	}

	md := renderEnrichmentMarkdown(enrichment)
	return os.WriteFile(filepath.Join(outDir, "doc_links.md"), []byte(md), 0o644)
}

func renderEnrichmentMarkdown(e KnowledgeEnrichment) string {
	var b strings.Builder
	b.WriteString("# Endpoint-Documentation Links (RAG)\n\n")
	b.WriteString(fmt.Sprintf("Total endpoints: **%d**\n", e.TotalEndpoints))
	b.WriteString(fmt.Sprintf("- Documented: %d\n", e.Documented))
	b.WriteString(fmt.Sprintf("- Partial: %d\n", e.Partial))
	b.WriteString(fmt.Sprintf("- Undocumented: %d\n\n", e.Undocumented))

	for _, link := range e.Links {
		icon := "❌"
		if link.Coverage == "documented" {
			icon = "✅"
		} else if link.Coverage == "partial" {
			icon = "⚠️"
		}
		b.WriteString(fmt.Sprintf("## %s `%s`\n\n", icon, link.Endpoint))

		if len(link.Links) == 0 {
			b.WriteString("No documentation found.\n\n")
			continue
		}

		b.WriteString("| Source | Score | Snippet |\n| --- | --- | --- |\n")
		for _, dl := range link.Links {
			snippet := strings.ReplaceAll(dl.Snippet, "\n", " ")
			if len(snippet) > 80 {
				snippet = snippet[:80] + "..."
			}
			b.WriteString(fmt.Sprintf("| `%s` | %.2f | %s |\n", dl.Source, dl.Score, snippet))
		}
		b.WriteString("\n")
	}

	return b.String()
}
