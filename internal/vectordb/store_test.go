package vectordb

import (
	"path/filepath"
	"testing"
)

func TestOpenAndClose(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	count, err := store.Count()
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

func TestUpsertAndSearch(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	records := []Record{
		{ID: "doc1#0", Source: "guide.md", ChunkIdx: 0, Content: "users endpoint", Embedding: []float64{1, 0, 0, 0}},
		{ID: "doc1#1", Source: "guide.md", ChunkIdx: 1, Content: "orders endpoint", Embedding: []float64{0, 1, 0, 0}},
		{ID: "doc2#0", Source: "api.md", ChunkIdx: 0, Content: "authentication", Embedding: []float64{0, 0, 1, 0}},
	}

	if err := store.UpsertBatch(records); err != nil {
		t.Fatalf("UpsertBatch failed: %v", err)
	}

	count, _ := store.Count()
	if count != 3 {
		t.Errorf("expected 3 records, got %d", count)
	}

	// Search for something similar to "users endpoint"
	results, err := store.Search([]float64{0.9, 0.1, 0, 0}, 2)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "doc1#0" {
		t.Errorf("expected doc1#0 as top result, got %s", results[0].ID)
	}
	if results[0].Score <= results[1].Score {
		t.Error("first result should have higher score")
	}
}

func TestUpsertReplace(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	rec := Record{ID: "doc1#0", Source: "guide.md", ChunkIdx: 0, Content: "old", Embedding: []float64{1, 0}}
	store.Upsert(rec)

	rec.Content = "new"
	rec.Embedding = []float64{0, 1}
	store.Upsert(rec)

	count, _ := store.Count()
	if count != 1 {
		t.Errorf("expected 1 after upsert replace, got %d", count)
	}
}

func TestDeleteBySource(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	store.Upsert(Record{ID: "a#0", Source: "a.md", ChunkIdx: 0, Content: "a", Embedding: []float64{1}})
	store.Upsert(Record{ID: "b#0", Source: "b.md", ChunkIdx: 0, Content: "b", Embedding: []float64{1}})

	store.DeleteBySource("a.md")

	count, _ := store.Count()
	if count != 1 {
		t.Errorf("expected 1 after delete, got %d", count)
	}
}

func TestPseudoEmbedder(t *testing.T) {
	emb := NewPseudoEmbedder(64)
	if emb.Dimension() != 64 {
		t.Errorf("expected dim 64, got %d", emb.Dimension())
	}

	vec1, err := emb.Embed("hello world")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	if len(vec1) != 64 {
		t.Errorf("expected 64-dim vector, got %d", len(vec1))
	}

	vec2, _ := emb.Embed("different text")
	same := true
	for i := range vec1 {
		if vec1[i] != vec2[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("different texts should produce different embeddings")
	}

	// Check unit norm
	var norm float64
	for _, v := range vec1 {
		norm += v * v
	}
	if norm < 0.99 || norm > 1.01 {
		t.Errorf("expected unit norm, got %f", norm)
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		a, b     []float64
		expected float64
	}{
		{[]float64{1, 0}, []float64{1, 0}, 1.0},
		{[]float64{1, 0}, []float64{0, 1}, 0.0},
		{[]float64{1, 0}, []float64{-1, 0}, -1.0},
		{[]float64{}, []float64{}, 0.0},
		{[]float64{1}, []float64{1, 2}, 0.0}, // different dims
	}
	for _, tt := range tests {
		got := cosineSimilarity(tt.a, tt.b)
		if got < tt.expected-0.01 || got > tt.expected+0.01 {
			t.Errorf("cosineSimilarity(%v, %v) = %f, want %f", tt.a, tt.b, got, tt.expected)
		}
	}
}
