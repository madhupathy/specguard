package diff

import "strings"

// ClassificationRule maps a change kind pattern to a severity and impact bonus.
type ClassificationRule struct {
	KindPrefix  string
	Severity    string
	ImpactBonus int
}

var defaultRules = []ClassificationRule{
	// Endpoint rules
	{KindPrefix: "endpoint.removed", Severity: "breaking", ImpactBonus: 15},
	{KindPrefix: "endpoint.added", Severity: "non_breaking", ImpactBonus: 1},
	{KindPrefix: "method.removed", Severity: "breaking", ImpactBonus: 10},
	{KindPrefix: "method.added", Severity: "non_breaking", ImpactBonus: 1},

	// Parameter rules
	{KindPrefix: "parameter.removed", Severity: "breaking", ImpactBonus: 8},
	{KindPrefix: "parameter.added", Severity: "non_breaking", ImpactBonus: 2},

	// Response rules
	{KindPrefix: "response.removed", Severity: "breaking", ImpactBonus: 8},
	{KindPrefix: "response.added", Severity: "non_breaking", ImpactBonus: 1},

	// Schema rules
	{KindPrefix: "schema.removed", Severity: "breaking", ImpactBonus: 10},
	{KindPrefix: "schema.added", Severity: "non_breaking", ImpactBonus: 1},
	{KindPrefix: "property.removed", Severity: "breaking", ImpactBonus: 8},
	{KindPrefix: "property.added", Severity: "non_breaking", ImpactBonus: 2},
	{KindPrefix: "property.type_changed", Severity: "breaking", ImpactBonus: 10},
	{KindPrefix: "field.became_required", Severity: "breaking", ImpactBonus: 8},
	{KindPrefix: "field.became_optional", Severity: "non_breaking", ImpactBonus: 2},

	// Security rules
	{KindPrefix: "security.removed", Severity: "breaking", ImpactBonus: 10},
	{KindPrefix: "security.added", Severity: "potential_breaking", ImpactBonus: 5},

	// Proto rules
	{KindPrefix: "service.removed", Severity: "breaking", ImpactBonus: 15},
	{KindPrefix: "service.added", Severity: "non_breaking", ImpactBonus: 1},

	// Snapshot-level entry rules
	{KindPrefix: "entry.removed", Severity: "breaking", ImpactBonus: 15},
	{KindPrefix: "entry.changed", Severity: "potential_breaking", ImpactBonus: 5},
	{KindPrefix: "entry.added", Severity: "non_breaking", ImpactBonus: 1},
}

// ClassifyResult applies classification rules to all changes in a Result,
// updating severity and recalculating the summary counts.
func ClassifyResult(result *Result) {
	result.Summary.Breaking = 0
	result.Summary.PotentialBreaking = 0
	result.Summary.NonBreaking = 0
	result.Summary.DocumentationOnly = 0

	for i := range result.Changes {
		classifyChange(&result.Changes[i])
		enhanceChange(&result.Changes[i])

		switch result.Changes[i].Severity {
		case "breaking":
			result.Summary.Breaking++
		case "potential_breaking":
			result.Summary.PotentialBreaking++
		case "documentation_only":
			result.Summary.DocumentationOnly++
		default:
			result.Summary.NonBreaking++
		}
	}
}

func classifyChange(ch *Change) {
	for _, rule := range defaultRules {
		if strings.HasPrefix(ch.Kind, rule.KindPrefix) {
			ch.Severity = rule.Severity
			if ch.Details == nil {
				ch.Details = map[string]string{}
			}
			ch.Details["rule"] = rule.KindPrefix
			return
		}
	}
}

func enhanceChange(ch *Change) {
	source := strings.ToLower(ch.Source)

	if strings.Contains(source, "deprecated") {
		ch.Severity = "non_breaking"
		if ch.Details == nil {
			ch.Details = map[string]string{}
		}
		ch.Details["deprecated"] = "true"
	}

	if strings.Contains(source, "security") || strings.Contains(source, "auth") {
		if ch.Details == nil {
			ch.Details = map[string]string{}
		}
		ch.Details["security_related"] = "true"
	}

	entryType := ""
	if ch.Details != nil {
		entryType = ch.Details["type"]
	}
	if entryType == "doc_index" || entryType == "markdown" {
		ch.Severity = "documentation_only"
	}
}
