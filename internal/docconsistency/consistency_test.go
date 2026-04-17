package docconsistency

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyze(t *testing.T) {
	dir := t.TempDir()

	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"paths": map[string]interface{}{
			"/users": map[string]interface{}{
				"get": map[string]interface{}{"operationId": "listUsers"},
			},
			"/orders": map[string]interface{}{
				"post": map[string]interface{}{"operationId": "createOrder"},
			},
		},
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{
				"User":  map[string]interface{}{"type": "object"},
				"Order": map[string]interface{}{"type": "object"},
			},
		},
	}
	specBytes, _ := json.Marshal(spec)
	specPath := filepath.Join(dir, "spec.json")
	os.WriteFile(specPath, specBytes, 0o644)

	chunks := []chunkRecord{
		{ID: "doc#0", Source: "guide.md", Content: "This guide covers the users endpoint and User model."},
	}
	chunksPath := filepath.Join(dir, "chunks.jsonl")
	f, _ := os.Create(chunksPath)
	for _, c := range chunks {
		raw, _ := json.Marshal(c)
		f.Write(raw)
		f.Write([]byte("\n"))
	}
	f.Close()

	report, err := Analyze(specPath, chunksPath)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if report.TotalIssues == 0 {
		t.Error("expected some issues (orders/Order not in docs)")
	}

	foundOrder := false
	for _, issue := range report.Issues {
		if issue.Path == "POST /orders" || issue.Path == "components.schemas.Order" {
			foundOrder = true
		}
	}
	if !foundOrder {
		t.Error("expected issue about undocumented orders endpoint or Order schema")
	}
}

func TestAnalyzeDocOnlyEndpoint(t *testing.T) {
	dir := t.TempDir()

	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"paths":   map[string]interface{}{},
	}
	specBytes, _ := json.Marshal(spec)
	specPath := filepath.Join(dir, "spec.json")
	os.WriteFile(specPath, specBytes, 0o644)

	chunks := []chunkRecord{
		{ID: "doc#0", Source: "guide.md", Content: "Use GET /api/v1/widgets to list all widgets."},
	}
	chunksPath := filepath.Join(dir, "chunks.jsonl")
	f, _ := os.Create(chunksPath)
	for _, c := range chunks {
		raw, _ := json.Marshal(c)
		f.Write(raw)
		f.Write([]byte("\n"))
	}
	f.Close()

	report, err := Analyze(specPath, chunksPath)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	found := false
	for _, issue := range report.Issues {
		if issue.Type == "doc_only_endpoint" {
			found = true
		}
	}
	if !found {
		t.Error("expected doc_only_endpoint issue for GET /api/v1/widgets")
	}
}

func TestWriteReport(t *testing.T) {
	dir := t.TempDir()
	report := Report{
		TotalIssues: 1,
		Issues: []Issue{
			{Type: "undocumented_endpoint", Severity: "medium", Path: "GET /foo", Description: "test", Remediation: "fix"},
		},
	}

	err := WriteReport(report, dir)
	if err != nil {
		t.Fatalf("WriteReport failed: %v", err)
	}

	path := filepath.Join(dir, "doc_consistency.md")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("doc_consistency.md not created: %v", err)
	}
}
