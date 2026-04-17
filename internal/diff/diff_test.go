package diff

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCompareIdenticalSnapshots(t *testing.T) {
	snap := Snapshot{
		GeneratedAt: "2025-01-01T00:00:00Z",
		Entries: []SnapshotEntry{
			{Type: "openapi", Source: "api/openapi.yaml", Output: "snapshot/openapi.normalized.json", SHA256: "abc123", SizeBytes: 1000},
		},
	}
	result := Compare(snap, snap)
	if result.Summary.TotalChanges != 0 {
		t.Errorf("expected 0 changes for identical snapshots, got %d", result.Summary.TotalChanges)
	}
}

func TestCompareDetectsRemoval(t *testing.T) {
	base := Snapshot{
		Entries: []SnapshotEntry{
			{Type: "openapi", Source: "api/openapi.yaml", Output: "out/openapi.json", SHA256: "abc"},
		},
	}
	head := Snapshot{Entries: []SnapshotEntry{}}
	result := Compare(base, head)
	if result.Summary.Removals != 1 {
		t.Errorf("expected 1 removal, got %d", result.Summary.Removals)
	}
	if result.Summary.Breaking != 1 {
		t.Errorf("expected 1 breaking, got %d", result.Summary.Breaking)
	}
}

func TestCompareDetectsAddition(t *testing.T) {
	base := Snapshot{Entries: []SnapshotEntry{}}
	head := Snapshot{
		Entries: []SnapshotEntry{
			{Type: "protobuf", Source: "proto/api.proto", Output: "out/proto.json", SHA256: "def"},
		},
	}
	result := Compare(base, head)
	if result.Summary.Additions != 1 {
		t.Errorf("expected 1 addition, got %d", result.Summary.Additions)
	}
}

func TestCompareDetectsMutation(t *testing.T) {
	base := Snapshot{
		Entries: []SnapshotEntry{
			{Type: "openapi", Source: "api/openapi.yaml", Output: "out/openapi.json", SHA256: "abc"},
		},
	}
	head := Snapshot{
		Entries: []SnapshotEntry{
			{Type: "openapi", Source: "api/openapi.yaml", Output: "out/openapi.json", SHA256: "def"},
		},
	}
	result := Compare(base, head)
	if result.Summary.Mutations != 1 {
		t.Errorf("expected 1 mutation, got %d", result.Summary.Mutations)
	}
}

func TestWriteAndLoadSnapshot(t *testing.T) {
	dir := t.TempDir()
	snap := Snapshot{
		GeneratedAt: "2025-01-01T00:00:00Z",
		EntryCount:  1,
		Entries: []SnapshotEntry{
			{Type: "openapi", Source: "api.yaml", Output: "out.json", SHA256: "abc", SizeBytes: 100},
		},
	}
	path := filepath.Join(dir, "snapshot.json")
	raw, _ := json.MarshalIndent(snap, "", "  ")
	os.WriteFile(path, raw, 0o644)

	loaded, err := LoadSnapshot(path)
	if err != nil {
		t.Fatalf("LoadSnapshot failed: %v", err)
	}
	if len(loaded.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(loaded.Entries))
	}
}

func TestResultWrite(t *testing.T) {
	dir := t.TempDir()
	result := Result{
		Changes: []Change{{ID: "test", Kind: "entry.added", Severity: "non_breaking", Source: "test"}},
		Summary: Summary{TotalChanges: 1, NonBreaking: 1},
	}
	changesPath := filepath.Join(dir, "changes.json")
	summaryPath := filepath.Join(dir, "summary.json")

	if err := result.Write(changesPath, summaryPath); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if _, err := os.Stat(changesPath); err != nil {
		t.Errorf("changes.json not created: %v", err)
	}
	if _, err := os.Stat(summaryPath); err != nil {
		t.Errorf("summary.json not created: %v", err)
	}
}

func TestClassifyResult(t *testing.T) {
	result := Result{
		Changes: []Change{
			{Kind: "endpoint.removed", Source: "/api/users"},
			{Kind: "endpoint.added", Source: "/api/posts"},
			{Kind: "parameter.removed", Source: "GET /api/users"},
			{Kind: "property.type_changed", Source: "components.schemas.User.properties.age"},
		},
		Summary: Summary{TotalChanges: 4},
	}
	ClassifyResult(&result)

	if result.Summary.Breaking != 3 {
		t.Errorf("expected 3 breaking, got %d", result.Summary.Breaking)
	}
	if result.Summary.NonBreaking != 1 {
		t.Errorf("expected 1 non_breaking, got %d", result.Summary.NonBreaking)
	}
	if result.Changes[0].Severity != "breaking" {
		t.Errorf("endpoint.removed should be breaking, got %s", result.Changes[0].Severity)
	}
	if result.Changes[1].Severity != "non_breaking" {
		t.Errorf("endpoint.added should be non_breaking, got %s", result.Changes[1].Severity)
	}
}

func TestDeepCompareOpenAPI(t *testing.T) {
	dir := t.TempDir()

	baseSpec := map[string]interface{}{
		"openapi": "3.0.0",
		"paths": map[string]interface{}{
			"/users": map[string]interface{}{
				"get": map[string]interface{}{
					"parameters": []interface{}{
						map[string]interface{}{"name": "limit", "in": "query"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "OK"},
					},
				},
			},
			"/old": map[string]interface{}{
				"delete": map[string]interface{}{},
			},
		},
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{
				"User": map[string]interface{}{
					"required":   []interface{}{"name"},
					"properties": map[string]interface{}{"name": map[string]interface{}{"type": "string"}, "age": map[string]interface{}{"type": "integer"}},
				},
			},
		},
	}

	headSpec := map[string]interface{}{
		"openapi": "3.0.0",
		"paths": map[string]interface{}{
			"/users": map[string]interface{}{
				"get": map[string]interface{}{
					"parameters": []interface{}{
						map[string]interface{}{"name": "limit", "in": "query"},
						map[string]interface{}{"name": "offset", "in": "query"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "OK"},
						"404": map[string]interface{}{"description": "Not Found"},
					},
				},
			},
			"/posts": map[string]interface{}{
				"get": map[string]interface{}{},
			},
		},
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{
				"User": map[string]interface{}{
					"required":   []interface{}{"name", "email"},
					"properties": map[string]interface{}{"name": map[string]interface{}{"type": "string"}, "email": map[string]interface{}{"type": "string"}},
				},
			},
		},
	}

	writeJSON := func(name string, data interface{}) string {
		p := filepath.Join(dir, name)
		raw, _ := json.Marshal(data)
		os.WriteFile(p, raw, 0o644)
		return p
	}

	basePath := writeJSON("base_openapi.json", baseSpec)
	headPath := writeJSON("head_openapi.json", headSpec)

	baseSnap := Snapshot{
		Entries: []SnapshotEntry{
			{Type: "openapi", Source: "api.yaml", Output: basePath, SHA256: "aaa", SizeBytes: 100},
		},
	}
	headSnap := Snapshot{
		Entries: []SnapshotEntry{
			{Type: "openapi", Source: "api.yaml", Output: headPath, SHA256: "bbb", SizeBytes: 200},
		},
	}

	result := DeepCompare(baseSnap, headSnap, dir)

	if result.Summary.TotalChanges == 0 {
		t.Fatal("expected deep changes, got 0")
	}

	kinds := map[string]int{}
	for _, ch := range result.Changes {
		kinds[ch.Kind]++
	}

	if kinds["endpoint.removed"] != 1 {
		t.Errorf("expected 1 endpoint.removed (/old), got %d", kinds["endpoint.removed"])
	}
	if kinds["endpoint.added"] != 1 {
		t.Errorf("expected 1 endpoint.added (/posts), got %d", kinds["endpoint.added"])
	}
	if kinds["parameter.added"] != 1 {
		t.Errorf("expected 1 parameter.added (offset), got %d", kinds["parameter.added"])
	}
	if kinds["response.added"] != 1 {
		t.Errorf("expected 1 response.added (404), got %d", kinds["response.added"])
	}
	if kinds["field.became_required"] != 1 {
		t.Errorf("expected 1 field.became_required (email), got %d", kinds["field.became_required"])
	}
	if kinds["property.removed"] != 1 {
		t.Errorf("expected 1 property.removed (age), got %d", kinds["property.removed"])
	}
	if kinds["property.added"] != 1 {
		t.Errorf("expected 1 property.added (email), got %d", kinds["property.added"])
	}
}
