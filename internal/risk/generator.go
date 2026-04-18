package risk

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// diffSummary mirrors internal/diff.Summary but keeps the dependency tree flat.
type diffSummary struct {
	GeneratedAt       string `json:"generated_at"`
	TotalChanges      int    `json:"total_changes"`
	Breaking          int    `json:"breaking"`
	PotentialBreaking int    `json:"potential_breaking"`
	NonBreaking       int    `json:"non_breaking"`
	DocumentationOnly int    `json:"documentation_only"`
	Additions         int    `json:"additions"`
	Removals          int    `json:"removals"`
	Mutations         int    `json:"mutations"`
}

// knowledgeModel captures the minimal fields needed from knowledge_model.json
type knowledgeModel struct {
	GeneratedAt string        `json:"generated_at"`
	Specs       []specEntry   `json:"specs"`
	DocIndex    *docIndexInfo `json:"doc_index"`
}

type specEntry struct {
	Type   string `json:"type"`
	Source string `json:"source"`
}

type docIndexInfo struct {
	SourceDir   string `json:"source_dir"`
	TotalChunks int    `json:"total_chunks"`
	TotalBytes  int64  `json:"total_bytes"`
}

// RiskFinding is a specific issue contributing to the risk score.
type RiskFinding struct {
	Category    string `json:"category"`
	Severity    string `json:"severity"`
	Path        string `json:"path"`
	Description string `json:"description"`
	Remediation string `json:"remediation"`
}

// ReportPayload captures the JSON report persisted to disk.
type ReportPayload struct {
	GeneratedAt string        `json:"generated_at"`
	Score       int           `json:"score"`
	Grade       string        `json:"grade"`
	Signals     Signals       `json:"signals"`
	Warnings    []string      `json:"warnings"`
	Findings    []RiskFinding `json:"findings,omitempty"`
}

// Signals tracks the numerical counts used for scoring.
type Signals struct {
	Breaking            int `json:"breaking"`
	PotentialBreaking   int `json:"potential_breaking"`
	NonBreaking         int `json:"non_breaking"`
	DocumentationOnly   int `json:"documentation_only"`
	Additions           int `json:"additions"`
	Removals            int `json:"removals"`
	Mutations           int `json:"mutations"`
	StandardsViolations int `json:"standards_violations"`
}

// Generate reads the diff summary + knowledge model and emits risk + protocol recommendation reports.
func Generate(diffSummaryPath, knowledgeModelPath, outDir string) error {
	return GenerateWithStandards(diffSummaryPath, knowledgeModelPath, outDir, 0)
}

// GenerateStandalone produces a risk report from standards violations alone (no diff needed).
func GenerateStandalone(outDir string, standardsViolations int, findings []RiskFinding) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create reports dir: %w", err)
	}
	reportPayload := buildRiskReport(diffSummary{}, standardsViolations)
	reportPayload.Findings = findings
	return writeRiskMarkdown(filepath.Join(outDir, "risk.md"), reportPayload)
}

// GenerateWithStandards is like Generate but also factors in standards violations.
func GenerateWithStandards(diffSummaryPath, knowledgeModelPath, outDir string, standardsViolations int) error {
	summary, err := loadSummary(diffSummaryPath)
	if err != nil {
		return err
	}
	model, err := loadKnowledge(knowledgeModelPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create reports dir: %w", err)
	}

	reportPayload := buildRiskReport(summary, standardsViolations)
	if err := writeRiskMarkdown(filepath.Join(outDir, "risk.md"), reportPayload); err != nil {
		return err
	}

	protocolPath := filepath.Join(outDir, "protocol_recommendation.md")
	if err := writeProtocolRecommendation(protocolPath, summary, model); err != nil {
		return err
	}

	return nil
}

func loadSummary(path string) (diffSummary, error) {
	var summary diffSummary
	raw, err := os.ReadFile(path)
	if err != nil {
		return summary, fmt.Errorf("read diff summary: %w", err)
	}
	if err := json.Unmarshal(raw, &summary); err != nil {
		return summary, fmt.Errorf("parse diff summary: %w", err)
	}
	return summary, nil
}

func loadKnowledge(path string) (knowledgeModel, error) {
	var km knowledgeModel
	raw, err := os.ReadFile(path)
	if err != nil {
		return km, fmt.Errorf("read knowledge model: %w", err)
	}
	if err := json.Unmarshal(raw, &km); err != nil {
		return km, fmt.Errorf("parse knowledge model: %w", err)
	}
	return km, nil
}

func buildRiskReport(summary diffSummary, standardsViolations int) ReportPayload {
	score := weightedScore(summary, standardsViolations)
	grade := scoreToGrade(score)
	warnings := collectWarnings(summary, standardsViolations)
	return ReportPayload{
		GeneratedAt: timestampOrNow(summary.GeneratedAt),
		Score:       score,
		Grade:       grade,
		Signals: Signals{
			Breaking:            summary.Breaking,
			PotentialBreaking:   summary.PotentialBreaking,
			NonBreaking:         summary.NonBreaking,
			DocumentationOnly:   summary.DocumentationOnly,
			Additions:           summary.Additions,
			Removals:            summary.Removals,
			Mutations:           summary.Mutations,
			StandardsViolations: standardsViolations,
		},
		Warnings: warnings,
	}
}

func weightedScore(summary diffSummary, standardsViolations int) int {
	// Note: breaking already includes field/param removals, enum changes, type changes.
	// Removals is NOT added separately to avoid double-counting.
	// Mutations covers non-breaking structural changes (additions, modifications).
	points := summary.Breaking*30 +
		summary.PotentialBreaking*12 +
		summary.Mutations*8 +
		summary.DocumentationOnly*3 +
		standardsViolations*2
	if points > 100 {
		points = 100
	}
	return points
}

func scoreToGrade(score int) string {
	switch {
	case score >= 85:
		return "CRITICAL"
	case score >= 65:
		return "HIGH"
	case score >= 40:
		return "MEDIUM"
	case score >= 20:
		return "LOW"
	default:
		return "INFO"
	}
}

func collectWarnings(summary diffSummary, standardsViolations int) []string {
	warnings := []string{}
	if summary.Breaking > 0 {
		warnings = append(warnings, fmt.Sprintf("%d breaking changes detected", summary.Breaking))
	}
	if summary.PotentialBreaking > 0 {
		warnings = append(warnings, fmt.Sprintf("%d potential breaking changes require manual review", summary.PotentialBreaking))
	}
	if summary.DocumentationOnly > 25 {
		warnings = append(warnings, "Documentation churn detected – ensure release notes stay aligned")
	}
	if standardsViolations > 0 {
		warnings = append(warnings, fmt.Sprintf("%d standards violations detected", standardsViolations))
	}
	return warnings
}

func writeRiskMarkdown(path string, payload ReportPayload) error {
	var b strings.Builder
	b.WriteString("# SpecGuard Risk Report\n\n")
	b.WriteString(fmt.Sprintf("Generated: %s\n", payload.GeneratedAt))
	b.WriteString(fmt.Sprintf("Overall Score: **%d/100 (%s)**\n\n", payload.Score, payload.Grade))
	b.WriteString("## Signal Breakdown\n")
	b.WriteString("| Signal | Count |\n| --- | --- |\n")
	b.WriteString(fmt.Sprintf("| Breaking | %d |\n", payload.Signals.Breaking))
	b.WriteString(fmt.Sprintf("| Potential Breaking | %d |\n", payload.Signals.PotentialBreaking))
	b.WriteString(fmt.Sprintf("| Non-breaking | %d |\n", payload.Signals.NonBreaking))
	b.WriteString(fmt.Sprintf("| Documentation-only | %d |\n", payload.Signals.DocumentationOnly))
	b.WriteString(fmt.Sprintf("| Additions | %d |\n", payload.Signals.Additions))
	b.WriteString(fmt.Sprintf("| Removals | %d |\n", payload.Signals.Removals))
	b.WriteString(fmt.Sprintf("| Mutations | %d |\n", payload.Signals.Mutations))
	b.WriteString(fmt.Sprintf("| Standards Violations | %d |\n", payload.Signals.StandardsViolations))

	if len(payload.Warnings) > 0 {
		b.WriteString("\n## Warnings\n")
		for _, warning := range payload.Warnings {
			b.WriteString(fmt.Sprintf("- %s\n", warning))
		}
	}

	if len(payload.Findings) > 0 {
		b.WriteString("\n## Detailed Findings\n\n")
		b.WriteString("| # | Category | Severity | Path | Description | Remediation |\n")
		b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
		for i, f := range payload.Findings {
			icon := "ℹ️"
			switch f.Severity {
			case "high", "critical":
				icon = "🔴"
			case "medium":
				icon = "🟡"
			case "low":
				icon = "🟢"
			}
			b.WriteString(fmt.Sprintf("| %d | %s %s | %s | `%s` | %s | %s |\n",
				i+1, icon, f.Category, f.Severity, f.Path, f.Description, f.Remediation))
		}
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeProtocolRecommendation(path string, summary diffSummary, model knowledgeModel) error {
	restSpec := hasSpec(model, "openapi")
	grpcSpec := hasSpec(model, "protobuf")
	restScore := summary.Additions + summary.Mutations + summary.DocumentationOnly
	grpcScore := summary.Breaking + summary.PotentialBreaking

	recommendation := "REST"
	reason := "OpenAPI surfaced more churn and is likely the primary contract."
	if grpcSpec && !restSpec {
		recommendation = "gRPC"
		reason = "Only protobuf services detected; prioritize gRPC clients."
	} else if grpcSpec && restSpec {
		if grpcScore > restScore {
			recommendation = "gRPC"
			reason = "Proto-facing changes dominate (breaking/potential)."
		} else {
			reason = "REST saw more additive/mutation activity and docs focus."
		}
	} else if !grpcSpec && restSpec {
		reason = "No protobuf descriptors detected; REST is the canonical interface."
	} else {
		recommendation = "Evaluate"
		reason = "Neither OpenAPI nor protobuf inputs found; cannot recommend a protocol."
	}

	if model.DocIndex != nil && model.DocIndex.TotalChunks > 0 {
		reason += fmt.Sprintf(" Documentation ingest captured %d chunks from %s.", model.DocIndex.TotalChunks, model.DocIndex.SourceDir)
	}

	content := fmt.Sprintf(`# Protocol Recommendation

**Recommended interface:** %s

Reasoning: %s

Signals considered:
- Breaking changes: %d
- Potential breaking changes: %d
- REST-oriented changes (additions/mutations/docs): %d
- gRPC-oriented changes: %d
`, recommendation, reason, summary.Breaking, summary.PotentialBreaking, restScore, grpcScore)

	return os.WriteFile(path, []byte(content), 0o644)
}

func hasSpec(model knowledgeModel, typ string) bool {
	for _, spec := range model.Specs {
		if spec.Type == typ {
			return true
		}
	}
	return false
}

func timestampOrNow(ts string) string {
	if ts != "" {
		return ts
	}
	return time.Now().UTC().Format(time.RFC3339Nano)
}
