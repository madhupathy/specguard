package standards

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

func TestAnalyzeMinimalSpec(t *testing.T) {
	dir := t.TempDir()
	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"info":    map[string]interface{}{"title": "Test", "version": "1.0.0"},
		"paths":   map[string]interface{}{},
	}
	path := writeSpec(t, dir, spec)

	report, err := Analyze(path)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	if report.TotalChecked != 10 {
		t.Errorf("expected 10 rules checked, got %d", report.TotalChecked)
	}
}

func TestRuleOperationIDMissing(t *testing.T) {
	dir := t.TempDir()
	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"info":    map[string]interface{}{"title": "Test", "version": "1.0.0"},
		"paths": map[string]interface{}{
			"/users": map[string]interface{}{
				"get": map[string]interface{}{
					"responses": map[string]interface{}{"200": map[string]interface{}{"description": "OK"}},
				},
			},
		},
	}
	path := writeSpec(t, dir, spec)

	report, err := Analyze(path)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	found := false
	for _, v := range report.Violations {
		if v.RuleID == "STD-004" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected STD-004 (operationId missing) violation")
	}
}

func TestRuleOperationIDPresent(t *testing.T) {
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
			},
		},
	}
	path := writeSpec(t, dir, spec)

	report, _ := Analyze(path)
	for _, v := range report.Violations {
		if v.RuleID == "STD-004" && v.Path == "GET /users" {
			t.Error("should not flag operationId when present")
		}
	}
}

func TestRuleNoAuth(t *testing.T) {
	dir := t.TempDir()
	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"info":    map[string]interface{}{"title": "Test", "version": "1.0.0"},
		"paths":   map[string]interface{}{},
	}
	path := writeSpec(t, dir, spec)

	report, _ := Analyze(path)
	found := false
	for _, v := range report.Violations {
		if v.RuleID == "STD-006" {
			found = true
		}
	}
	if !found {
		t.Error("expected STD-006 (auth not documented) violation")
	}
}

func TestRuleVersionMissing(t *testing.T) {
	dir := t.TempDir()
	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"info":    map[string]interface{}{"title": "Test"},
		"paths":   map[string]interface{}{"/foo": map[string]interface{}{}},
	}
	path := writeSpec(t, dir, spec)

	report, _ := Analyze(path)
	found := false
	for _, v := range report.Violations {
		if v.RuleID == "STD-003" {
			found = true
		}
	}
	if !found {
		t.Error("expected STD-003 (versioning) violation")
	}
}

func TestRuleNoErrorResponses(t *testing.T) {
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
			},
		},
	}
	path := writeSpec(t, dir, spec)

	report, _ := Analyze(path)
	found := false
	for _, v := range report.Violations {
		if v.RuleID == "STD-001" {
			found = true
		}
	}
	if !found {
		t.Error("expected STD-001 (no error responses) violation")
	}
}

func TestWriteReport(t *testing.T) {
	dir := t.TempDir()
	report := Report{
		TotalChecked:   10,
		TotalViolation: 2,
		Violations: []Violation{
			{RuleID: "STD-001", RuleName: "test", Severity: "medium", Path: "/foo", Description: "desc", Remediation: "fix"},
			{RuleID: "STD-002", RuleName: "test2", Severity: "low", Path: "/bar", Description: "desc2", Remediation: "fix2"},
		},
	}

	err := WriteReport(report, dir)
	if err != nil {
		t.Fatalf("WriteReport failed: %v", err)
	}

	for _, name := range []string{"standards.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s not created: %v", name, err)
		}
	}
}
