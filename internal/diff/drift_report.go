package diff

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteDriftReport generates a drift.md from the diff result.
func WriteDriftReport(result Result, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create reports dir: %w", err)
	}

	md := renderDriftMarkdown(result)
	return os.WriteFile(filepath.Join(outDir, "drift.md"), []byte(md), 0o644)
}

func renderDriftMarkdown(result Result) string {
	var b strings.Builder
	b.WriteString("# SpecGuard Drift Report\n\n")
	b.WriteString(fmt.Sprintf("Generated: %s\n\n", result.Summary.GeneratedAt))
	b.WriteString(fmt.Sprintf("Total changes: **%d**\n\n", result.Summary.TotalChanges))

	if len(result.Changes) == 0 {
		b.WriteString("No drift detected between base and head snapshots.\n")
		return b.String()
	}

	b.WriteString("## Summary\n\n")
	b.WriteString("| Metric | Count |\n| --- | --- |\n")
	b.WriteString(fmt.Sprintf("| Breaking | %d |\n", result.Summary.Breaking))
	b.WriteString(fmt.Sprintf("| Potential Breaking | %d |\n", result.Summary.PotentialBreaking))
	b.WriteString(fmt.Sprintf("| Non-breaking | %d |\n", result.Summary.NonBreaking))
	b.WriteString(fmt.Sprintf("| Documentation-only | %d |\n", result.Summary.DocumentationOnly))
	b.WriteString(fmt.Sprintf("| Additions | %d |\n", result.Summary.Additions))
	b.WriteString(fmt.Sprintf("| Removals | %d |\n", result.Summary.Removals))
	b.WriteString(fmt.Sprintf("| Mutations | %d |\n", result.Summary.Mutations))

	breaking := filterBySeverity(result.Changes, "breaking")
	if len(breaking) > 0 {
		b.WriteString("\n## Breaking Changes\n\n")
		b.WriteString("| Kind | Source | Details |\n| --- | --- | --- |\n")
		for _, ch := range breaking {
			b.WriteString(fmt.Sprintf("| %s | `%s` | %s |\n", ch.Kind, ch.Source, formatDetails(ch.Details)))
		}
	}

	potential := filterBySeverity(result.Changes, "potential_breaking")
	if len(potential) > 0 {
		b.WriteString("\n## Potential Breaking Changes\n\n")
		b.WriteString("| Kind | Source | Details |\n| --- | --- | --- |\n")
		for _, ch := range potential {
			b.WriteString(fmt.Sprintf("| %s | `%s` | %s |\n", ch.Kind, ch.Source, formatDetails(ch.Details)))
		}
	}

	nonBreaking := filterBySeverity(result.Changes, "non_breaking")
	if len(nonBreaking) > 0 {
		b.WriteString("\n## Non-breaking Changes\n\n")
		b.WriteString("| Kind | Source | Details |\n| --- | --- | --- |\n")
		for _, ch := range nonBreaking {
			b.WriteString(fmt.Sprintf("| %s | `%s` | %s |\n", ch.Kind, ch.Source, formatDetails(ch.Details)))
		}
	}

	docOnly := filterBySeverity(result.Changes, "documentation_only")
	if len(docOnly) > 0 {
		b.WriteString("\n## Documentation-only Changes\n\n")
		b.WriteString("| Kind | Source |\n| --- | --- |\n")
		for _, ch := range docOnly {
			b.WriteString(fmt.Sprintf("| %s | `%s` |\n", ch.Kind, ch.Source))
		}
	}

	return b.String()
}

func filterBySeverity(changes []Change, severity string) []Change {
	var out []Change
	for _, ch := range changes {
		if ch.Severity == severity {
			out = append(out, ch)
		}
	}
	return out
}

func formatDetails(details map[string]string) string {
	if len(details) == 0 {
		return ""
	}
	parts := []string{}
	for k, v := range details {
		if k == "type" || k == "output" || k == "rule" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, ", ")
}
