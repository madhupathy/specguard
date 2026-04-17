package docindex

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildWithMarkdownFiles(t *testing.T) {
	docsDir := t.TempDir()
	outDir := t.TempDir()

	os.WriteFile(filepath.Join(docsDir, "guide.md"), []byte("# API Guide\n\nThis is a test document with enough words to generate at least one chunk of content for the document indexer to process correctly."), 0o644)
	os.WriteFile(filepath.Join(docsDir, "notes.txt"), []byte("Some plain text notes about the API endpoints and their usage patterns."), 0o644)

	summary, err := Build(context.Background(), docsDir, outDir)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if len(summary.Documents) != 2 {
		t.Errorf("expected 2 documents, got %d", len(summary.Documents))
	}
	if summary.TotalChunks == 0 {
		t.Error("expected at least 1 chunk")
	}
	if summary.TotalEmbeddings != summary.TotalChunks {
		t.Errorf("embeddings (%d) should equal chunks (%d)", summary.TotalEmbeddings, summary.TotalChunks)
	}

	for _, name := range []string{"chunks.jsonl", "embeddings.jsonl", "manifest.json"} {
		path := filepath.Join(outDir, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s not created: %v", name, err)
		}
	}

	if summary.ManifestPath == "" {
		t.Error("ManifestPath should be set")
	}
	if summary.Digest == "" {
		t.Error("Digest should be set")
	}
	if summary.TotalBytes == 0 {
		t.Error("TotalBytes should be > 0")
	}
}

func TestBuildEmptyDir(t *testing.T) {
	docsDir := t.TempDir()
	outDir := t.TempDir()

	summary, err := Build(context.Background(), docsDir, outDir)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if summary.TotalChunks != 0 {
		t.Errorf("expected 0 chunks for empty dir, got %d", summary.TotalChunks)
	}
}

func TestBuildNonexistentDir(t *testing.T) {
	_, err := Build(context.Background(), "/nonexistent/path", t.TempDir())
	if err == nil {
		t.Error("expected error for nonexistent dir")
	}
}

func TestBuildEmptyDocsDir(t *testing.T) {
	_, err := Build(context.Background(), "", t.TempDir())
	if err == nil {
		t.Error("expected error for empty docs dir")
	}
}

func TestChunkText(t *testing.T) {
	text := "one two three four five six seven eight nine ten"
	chunks := chunkText(text, 3)
	if len(chunks) != 4 {
		t.Errorf("expected 4 chunks with maxWords=3, got %d", len(chunks))
	}
	if chunks[0] != "one two three" {
		t.Errorf("unexpected first chunk: %s", chunks[0])
	}
	if chunks[3] != "ten" {
		t.Errorf("unexpected last chunk: %s", chunks[3])
	}
}

func TestChunkTextEmpty(t *testing.T) {
	chunks := chunkText("", 10)
	if chunks != nil {
		t.Errorf("expected nil for empty text, got %v", chunks)
	}
}

func TestSupportedDocExtension(t *testing.T) {
	tests := []struct {
		ext      string
		expected bool
	}{
		{".md", true},
		{".markdown", true},
		{".txt", true},
		{".pdf", true},
		{".go", false},
		{".json", false},
		{".yaml", false},
	}
	for _, tt := range tests {
		got := supportedDocExtension(tt.ext)
		if got != tt.expected {
			t.Errorf("supportedDocExtension(%q) = %v, want %v", tt.ext, got, tt.expected)
		}
	}
}

func TestPseudoEmbedding(t *testing.T) {
	vec := pseudoEmbedding("test text")
	if len(vec) != 8 {
		t.Errorf("expected 8-dim vector, got %d", len(vec))
	}
	for _, v := range vec {
		if v < 0 {
			t.Errorf("embedding values should be non-negative, got %f", v)
		}
	}

	vec2 := pseudoEmbedding("different text")
	same := true
	for i := range vec {
		if vec[i] != vec2[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("different texts should produce different embeddings")
	}
}
