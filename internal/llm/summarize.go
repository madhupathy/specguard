package llm

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SectionSummaries holds LLM-generated narrative text for each report section.
type SectionSummaries struct {
	Executive      string
	Risk           string
	Standards      string
	DocConsistency string
	Protocol       string
	Enrichment     string
}

// Summarizer generates narrative summaries using an LLM provider.
type Summarizer struct {
	provider Provider
}

// NewSummarizer creates a Summarizer. Returns nil if provider is nil.
func NewSummarizer(p Provider) *Summarizer {
	if p == nil {
		return nil
	}
	return &Summarizer{provider: p}
}

// SummarizeRisk generates a narrative for the risk report.
func (s *Summarizer) SummarizeRisk(score int, grade string, findings []string) string {
	if s == nil {
		return ""
	}
	prompt := fmt.Sprintf(`Summarize this API risk assessment in 2-3 sentences for a technical lead:
- Risk score: %d/100 (grade: %s)
- Key findings:
%s

Be concise and actionable. Do not use markdown headers.`, score, grade, strings.Join(findings, "\n"))

	return s.generate(prompt)
}

// SummarizeStandards generates a narrative for the standards report.
func (s *Summarizer) SummarizeStandards(totalChecked, totalViolations int, violations []string) string {
	if s == nil {
		return ""
	}
	prompt := fmt.Sprintf(`Summarize this API standards compliance check in 2-3 sentences:
- Rules checked: %d
- Violations found: %d
- Violations:
%s

Be concise and actionable. Do not use markdown headers.`, totalChecked, totalViolations, strings.Join(violations, "\n"))

	return s.generate(prompt)
}

// SummarizeDocConsistency generates a narrative for the doc consistency report.
func (s *Summarizer) SummarizeDocConsistency(totalIssues int, undocumented []string) string {
	if s == nil {
		return ""
	}
	sample := undocumented
	if len(sample) > 5 {
		sample = sample[:5]
	}
	prompt := fmt.Sprintf(`Summarize this API documentation consistency check in 2-3 sentences:
- Total issues: %d
- Sample undocumented endpoints: %s

Be concise and actionable. Do not use markdown headers.`, totalIssues, strings.Join(sample, ", "))

	return s.generate(prompt)
}

// SummarizeProtocol generates a narrative for the protocol recommendation report.
func (s *Summarizer) SummarizeProtocol(rest, grpc, either int) string {
	if s == nil {
		return ""
	}
	total := rest + grpc + either
	prompt := fmt.Sprintf(`Summarize this API protocol recommendation analysis in 2-3 sentences:
- Total endpoints: %d
- REST preferred: %d (%.0f%%)
- gRPC preferred: %d (%.0f%%)
- Either viable: %d (%.0f%%)

Be concise and actionable. Do not use markdown headers.`,
		total,
		rest, pct(rest, total),
		grpc, pct(grpc, total),
		either, pct(either, total))

	return s.generate(prompt)
}

// SummarizeEnrichment generates a narrative for the enrichment report.
func (s *Summarizer) SummarizeEnrichment(total, enriched, chunks int) string {
	if s == nil {
		return ""
	}
	prompt := fmt.Sprintf(`Summarize this API documentation enrichment in 2-3 sentences:
- Total endpoints: %d
- Enriched with docs: %d (%.0f%% coverage)
- Doc chunks matched: %d

Be concise and actionable. Do not use markdown headers.`,
		total, enriched, pct(enriched, total), chunks)

	return s.generate(prompt)
}

// GenerateExecutiveSummary creates a top-level narrative combining all sections.
func (s *Summarizer) GenerateExecutiveSummary(sections SectionSummaries) string {
	if s == nil {
		return ""
	}
	var parts []string
	if sections.Risk != "" {
		parts = append(parts, "Risk: "+sections.Risk)
	}
	if sections.Standards != "" {
		parts = append(parts, "Standards: "+sections.Standards)
	}
	if sections.DocConsistency != "" {
		parts = append(parts, "Documentation: "+sections.DocConsistency)
	}
	if sections.Protocol != "" {
		parts = append(parts, "Protocol: "+sections.Protocol)
	}
	if sections.Enrichment != "" {
		parts = append(parts, "Enrichment: "+sections.Enrichment)
	}

	prompt := fmt.Sprintf(`Write a 3-4 sentence executive summary for an API governance report based on these section summaries:

%s

The summary should be suitable for a VP of Engineering. Be concise, highlight the most important action items, and provide an overall health assessment. Do not use markdown headers.`,
		strings.Join(parts, "\n\n"))

	return s.generate(prompt)
}

func (s *Summarizer) generate(prompt string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := s.provider.Generate(ctx, prompt)
	if err != nil {
		return fmt.Sprintf("(LLM summary unavailable: %v)", err)
	}
	return strings.TrimSpace(result)
}

func pct(part, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}
