package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeSpec(t *testing.T, dir string, spec map[string]interface{}) string {
	t.Helper()
	path := filepath.Join(dir, "spec.json")
	raw, _ := json.Marshal(spec)
	os.WriteFile(path, raw, 0o644)
	return path
}

func TestAnalyzeCRUDEndpoints(t *testing.T) {
	dir := t.TempDir()
	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"info":    map[string]interface{}{"title": "Test", "version": "1.0.0"},
		"paths": map[string]interface{}{
			"/users": map[string]interface{}{
				"get": map[string]interface{}{
					"operationId": "listUsers",
					"responses":   map[string]interface{}{"200": map[string]interface{}{"description": "OK"}},
				},
				"post": map[string]interface{}{
					"operationId": "createUser",
					"responses":   map[string]interface{}{"201": map[string]interface{}{"description": "Created"}},
				},
			},
			"/users/{id}": map[string]interface{}{
				"put": map[string]interface{}{
					"operationId": "updateUser",
					"responses":   map[string]interface{}{"200": map[string]interface{}{"description": "OK"}},
				},
				"delete": map[string]interface{}{
					"operationId": "deleteUser",
					"responses":   map[string]interface{}{"204": map[string]interface{}{"description": "Deleted"}},
				},
			},
		},
	}
	path := writeSpec(t, dir, spec)

	report, err := AnalyzeSpec(path, false)
	if err != nil {
		t.Fatalf("AnalyzeSpec failed: %v", err)
	}

	if report.TotalEndpoints != 4 {
		t.Errorf("expected 4 endpoints, got %d", report.TotalEndpoints)
	}

	// Without proto, CRUD endpoints should lean REST
	if report.RESTPreferred == 0 {
		t.Error("expected at least some REST-preferred endpoints for CRUD pattern")
	}
}

func TestAnalyzeStreamingEndpoint(t *testing.T) {
	dir := t.TempDir()
	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"info":    map[string]interface{}{"title": "Test", "version": "1.0.0"},
		"paths": map[string]interface{}{
			"/events/stream": map[string]interface{}{
				"get": map[string]interface{}{
					"operationId": "streamEvents",
					"description": "Stream real-time events via SSE",
					"responses":   map[string]interface{}{"200": map[string]interface{}{"description": "OK"}},
				},
			},
		},
	}
	path := writeSpec(t, dir, spec)

	report, err := AnalyzeSpec(path, true)
	if err != nil {
		t.Fatalf("AnalyzeSpec failed: %v", err)
	}

	if report.TotalEndpoints != 1 {
		t.Fatalf("expected 1 endpoint, got %d", report.TotalEndpoints)
	}

	rec := report.Recommendations[0]
	if rec.Protocol != "gRPC" {
		t.Errorf("expected gRPC for streaming endpoint, got %s", rec.Protocol)
	}
	if rec.Signals["streaming_hint"] != "true" {
		t.Error("expected streaming_hint signal")
	}
}

func TestAnalyzeBatchEndpoint(t *testing.T) {
	dir := t.TempDir()
	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"info":    map[string]interface{}{"title": "Test", "version": "1.0.0"},
		"paths": map[string]interface{}{
			"/svc/batch/process": map[string]interface{}{
				"post": map[string]interface{}{
					"operationId": "batchProcess",
					"description": "Batch process items",
					"responses":   map[string]interface{}{"200": map[string]interface{}{"description": "OK"}},
				},
			},
		},
	}
	path := writeSpec(t, dir, spec)

	report, err := AnalyzeSpec(path, true)
	if err != nil {
		t.Fatalf("AnalyzeSpec failed: %v", err)
	}

	rec := report.Recommendations[0]
	if rec.Protocol != "gRPC" {
		t.Errorf("expected gRPC for batch+internal endpoint, got %s", rec.Protocol)
	}
}

func TestAnalyzeFileUpload(t *testing.T) {
	dir := t.TempDir()
	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"info":    map[string]interface{}{"title": "Test", "version": "1.0.0"},
		"paths": map[string]interface{}{
			"/files": map[string]interface{}{
				"post": map[string]interface{}{
					"operationId": "uploadFile",
					"requestBody": map[string]interface{}{
						"content": map[string]interface{}{
							"multipart/form-data": map[string]interface{}{},
						},
					},
					"responses": map[string]interface{}{"201": map[string]interface{}{"description": "Created"}},
				},
			},
		},
	}
	path := writeSpec(t, dir, spec)

	report, err := AnalyzeSpec(path, true)
	if err != nil {
		t.Fatalf("AnalyzeSpec failed: %v", err)
	}

	rec := report.Recommendations[0]
	if rec.Protocol != "REST" {
		t.Errorf("expected REST for file upload, got %s", rec.Protocol)
	}
}

func TestWriteReport(t *testing.T) {
	dir := t.TempDir()
	report := Report{
		TotalEndpoints: 2,
		RESTPreferred:  1,
		GRPCPreferred:  1,
		Recommendations: []Recommendation{
			{Endpoint: "GET /users", Protocol: "REST", Confidence: 0.8, Rationale: "CRUD"},
			{Endpoint: "POST /svc/batch", Protocol: "gRPC", Confidence: 0.7, Rationale: "batch"},
		},
	}

	err := WriteReport(report, dir)
	if err != nil {
		t.Fatalf("WriteReport failed: %v", err)
	}

	for _, name := range []string{"protocol_recommendation.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s not created: %v", name, err)
		}
	}
}

func TestClamp(t *testing.T) {
	if clamp(0.3, 0.5, 0.95) != 0.5 {
		t.Error("clamp below min")
	}
	if clamp(0.99, 0.5, 0.95) != 0.95 {
		t.Error("clamp above max")
	}
	if clamp(0.7, 0.5, 0.95) != 0.7 {
		t.Error("clamp in range")
	}
}
