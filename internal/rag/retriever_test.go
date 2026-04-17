package rag

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/specguard/specguard/internal/vectordb"
)

func setupStore(t *testing.T) (*vectordb.Store, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := vectordb.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	return store, dir
}

func TestIngestChunksToStore(t *testing.T) {
	store, dir := setupStore(t)
	defer store.Close()

	chunks := []map[string]interface{}{
		{"id": "doc#0", "source": "guide.md", "chunk_index": 0, "content": "The users endpoint allows listing all users"},
		{"id": "doc#1", "source": "guide.md", "chunk_index": 1, "content": "Authentication uses OAuth2 bearer tokens"},
		{"id": "doc#2", "source": "api.md", "chunk_index": 0, "content": "Orders can be created via POST /orders"},
	}

	chunksPath := filepath.Join(dir, "chunks.jsonl")
	f, _ := os.Create(chunksPath)
	for _, c := range chunks {
		raw, _ := json.Marshal(c)
		f.Write(raw)
		f.Write([]byte("\n"))
	}
	f.Close()

	embedder := vectordb.NewPseudoEmbedder(32)
	count, err := IngestChunksToStore(chunksPath, store, embedder)
	if err != nil {
		t.Fatalf("IngestChunksToStore: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 ingested, got %d", count)
	}

	dbCount, _ := store.Count()
	if dbCount != 3 {
		t.Errorf("expected 3 in store, got %d", dbCount)
	}
}

func TestLinkEndpoints(t *testing.T) {
	store, dir := setupStore(t)
	defer store.Close()

	embedder := vectordb.NewPseudoEmbedder(32)

	// Ingest some chunks
	chunks := []map[string]interface{}{
		{"id": "doc#0", "source": "guide.md", "chunk_index": 0, "content": "The users endpoint allows listing all users"},
		{"id": "doc#1", "source": "guide.md", "chunk_index": 1, "content": "Authentication uses OAuth2 bearer tokens"},
	}
	chunksPath := filepath.Join(dir, "chunks.jsonl")
	f, _ := os.Create(chunksPath)
	for _, c := range chunks {
		raw, _ := json.Marshal(c)
		f.Write(raw)
		f.Write([]byte("\n"))
	}
	f.Close()

	IngestChunksToStore(chunksPath, store, embedder)

	retriever := NewRetriever(store, embedder, 3)
	enrichment, err := retriever.LinkEndpoints([]string{"GET /users", "POST /orders"})
	if err != nil {
		t.Fatalf("LinkEndpoints: %v", err)
	}

	if enrichment.TotalEndpoints != 2 {
		t.Errorf("expected 2 endpoints, got %d", enrichment.TotalEndpoints)
	}

	if len(enrichment.Links) != 2 {
		t.Errorf("expected 2 links, got %d", len(enrichment.Links))
	}
}

func TestWriteEnrichment(t *testing.T) {
	dir := t.TempDir()
	enrichment := KnowledgeEnrichment{
		TotalEndpoints: 2,
		Documented:     1,
		Undocumented:   1,
		Links: []EndpointDocLink{
			{
				Endpoint: "GET /users",
				Coverage: "documented",
				Links: []DocLink{
					{ChunkID: "doc#0", Source: "guide.md", Score: 0.85, Snippet: "users endpoint"},
				},
			},
			{
				Endpoint: "POST /orders",
				Coverage: "undocumented",
				Links:    []DocLink{},
			},
		},
	}

	err := WriteEnrichment(enrichment, dir)
	if err != nil {
		t.Fatalf("WriteEnrichment: %v", err)
	}

	for _, name := range []string{"doc_links.json", "doc_links.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s not created: %v", name, err)
		}
	}
}
