package risk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWeightedScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  diffSummary
		stdViol  int
		expected int
	}{
		{"zero changes", diffSummary{}, 0, 0},
		{"one breaking", diffSummary{Breaking: 1}, 0, 30},
		{"one potential", diffSummary{PotentialBreaking: 1}, 0, 15},
		{"capped at 100", diffSummary{Breaking: 5}, 0, 100},
		{"mixed", diffSummary{Breaking: 1, PotentialBreaking: 2, Mutations: 1, DocumentationOnly: 2}, 0, 80},
		{"with standards", diffSummary{Breaking: 1}, 5, 45},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := weightedScore(tt.summary, tt.stdViol)
			if got != tt.expected {
				t.Errorf("weightedScore() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestScoreToGrade(t *testing.T) {
	tests := []struct {
		score int
		grade string
	}{
		{0, "INFO"},
		{19, "INFO"},
		{20, "LOW"},
		{39, "LOW"},
		{40, "MEDIUM"},
		{64, "MEDIUM"},
		{65, "HIGH"},
		{84, "HIGH"},
		{85, "CRITICAL"},
		{100, "CRITICAL"},
	}
	for _, tt := range tests {
		got := scoreToGrade(tt.score)
		if got != tt.grade {
			t.Errorf("scoreToGrade(%d) = %s, want %s", tt.score, got, tt.grade)
		}
	}
}

func TestCollectWarnings(t *testing.T) {
	summary := diffSummary{Breaking: 2, PotentialBreaking: 3}
	warnings := collectWarnings(summary, 0)
	if len(warnings) != 2 {
		t.Errorf("expected 2 warnings, got %d", len(warnings))
	}

	summary2 := diffSummary{DocumentationOnly: 30}
	warnings2 := collectWarnings(summary2, 5)
	if len(warnings2) != 2 {
		t.Errorf("expected 2 warnings (doc churn + standards), got %d", len(warnings2))
	}
}

func TestBuildRiskReport(t *testing.T) {
	summary := diffSummary{
		GeneratedAt:       "2025-01-01T00:00:00Z",
		Breaking:          2,
		PotentialBreaking: 1,
		Mutations:         1,
	}
	report := buildRiskReport(summary, 0)
	if report.Score != 85 {
		t.Errorf("expected score 85, got %d", report.Score)
	}
	if report.Grade != "CRITICAL" {
		t.Errorf("expected CRITICAL, got %s", report.Grade)
	}
	if report.Signals.Breaking != 2 {
		t.Errorf("expected 2 breaking signals, got %d", report.Signals.Breaking)
	}
}

func TestGenerate(t *testing.T) {
	dir := t.TempDir()

	summary := diffSummary{
		GeneratedAt:  "2025-01-01T00:00:00Z",
		TotalChanges: 3,
		Breaking:     1,
		Additions:    2,
	}
	summaryBytes, _ := json.MarshalIndent(summary, "", "  ")
	summaryPath := filepath.Join(dir, "summary.json")
	os.WriteFile(summaryPath, summaryBytes, 0o644)

	km := knowledgeModel{
		GeneratedAt: "2025-01-01T00:00:00Z",
		Specs:       []specEntry{{Type: "openapi", Source: "api.yaml"}, {Type: "protobuf", Source: "proto/"}},
		DocIndex:    &docIndexInfo{SourceDir: "/docs", TotalChunks: 50, TotalBytes: 10000},
	}
	kmBytes, _ := json.MarshalIndent(km, "", "  ")
	kmPath := filepath.Join(dir, "knowledge.json")
	os.WriteFile(kmPath, kmBytes, 0o644)

	outDir := filepath.Join(dir, "reports")
	err := Generate(summaryPath, kmPath, outDir)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	for _, name := range []string{"risk.md", "protocol_recommendation.md"} {
		path := filepath.Join(outDir, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s not created: %v", name, err)
		}
	}

	riskMD, _ := os.ReadFile(filepath.Join(outDir, "risk.md"))
	content := string(riskMD)
	if strings.Contains(content, "\\n") {
		t.Error("risk.md contains literal \\n instead of newlines")
	}
	if !strings.Contains(content, "# SpecGuard Risk Report") {
		t.Error("risk.md missing header")
	}

	protoRec, _ := os.ReadFile(filepath.Join(outDir, "protocol_recommendation.md"))
	if !strings.Contains(string(protoRec), "Documentation ingest captured 50 chunks") {
		t.Error("protocol_recommendation.md missing doc index info")
	}
}
