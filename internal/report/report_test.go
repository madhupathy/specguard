package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerate(t *testing.T) {
	dir := t.TempDir()

	mani := Manifest{
		Version:     1,
		GeneratedAt: "2025-01-01T00:00:00Z",
		Entries: []ManifestEntry{
			{Type: "openapi", Source: "api.yaml", Output: "out/openapi.json", SizeBytes: 2048, SHA256: "abc"},
			{Type: "protobuf", Source: "proto/", Output: "out/proto.json", SizeBytes: 4096, SHA256: "def", Count: 10},
		},
	}
	maniBytes, _ := json.MarshalIndent(mani, "", "  ")
	maniPath := filepath.Join(dir, "manifest.json")
	os.WriteFile(maniPath, maniBytes, 0o644)

	outPath := filepath.Join(dir, "report.json")
	err := Generate(maniPath, outPath)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}

	var summary Summary
	if err := json.Unmarshal(raw, &summary); err != nil {
		t.Fatalf("parse report: %v", err)
	}

	if summary.Totals.Entries != 2 {
		t.Errorf("expected 2 entries, got %d", summary.Totals.Entries)
	}
	if summary.Totals.OpenAPI != 1 {
		t.Errorf("expected 1 openapi, got %d", summary.Totals.OpenAPI)
	}
	if summary.Totals.Protobuf != 1 {
		t.Errorf("expected 1 protobuf, got %d", summary.Totals.Protobuf)
	}
	if len(summary.Entries) != 2 {
		t.Errorf("expected 2 summary entries, got %d", len(summary.Entries))
	}
	if summary.Entries[0].SizeKB != 2 {
		t.Errorf("expected 2 KB, got %d", summary.Entries[0].SizeKB)
	}
}

func TestGenerateNoOpenAPI(t *testing.T) {
	dir := t.TempDir()

	mani := Manifest{
		Version:     1,
		GeneratedAt: "2025-01-01T00:00:00Z",
		Entries: []ManifestEntry{
			{Type: "protobuf", Source: "proto/", Output: "out/proto.json", SizeBytes: 4096, SHA256: "def"},
		},
	}
	maniBytes, _ := json.MarshalIndent(mani, "", "  ")
	maniPath := filepath.Join(dir, "manifest.json")
	os.WriteFile(maniPath, maniBytes, 0o644)

	outPath := filepath.Join(dir, "report.json")
	err := Generate(maniPath, outPath)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	raw, _ := os.ReadFile(outPath)
	var summary Summary
	json.Unmarshal(raw, &summary)

	if len(summary.Warnings) == 0 {
		t.Error("expected warnings about missing OpenAPI")
	}
}

func TestGenerateMissingManifest(t *testing.T) {
	err := Generate("/nonexistent/manifest.json", "/tmp/out.json")
	if err == nil {
		t.Error("expected error for missing manifest")
	}
}
