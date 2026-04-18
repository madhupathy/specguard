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
	{KindPrefix: "parameter.location_changed", Severity: "breaking", ImpactBonus: 10},
	{KindPrefix: "parameter.type_changed", Severity: "breaking", ImpactBonus: 9},
	{KindPrefix: "parameter.became_required", Severity: "breaking", ImpactBonus: 8},

	// Request body rules
	{KindPrefix: "request_body.removed", Severity: "breaking", ImpactBonus: 10},
	{KindPrefix: "request_body.added", Severity: "potential_breaking", ImpactBonus: 4},
	{KindPrefix: "request_body.became_required", Severity: "breaking", ImpactBonus: 8},

	// Response rules
	{KindPrefix: "response.removed", Severity: "breaking", ImpactBonus: 8},
	{KindPrefix: "response.added", Severity: "non_breaking", ImpactBonus: 1},

	// Schema rules (component + inline)
	{KindPrefix: "schema.removed", Severity: "breaking", ImpactBonus: 10},
	{KindPrefix: "schema.added", Severity: "non_breaking", ImpactBonus: 1},
	{KindPrefix: "property.removed", Severity: "breaking", ImpactBonus: 8},
	{KindPrefix: "property.added", Severity: "non_breaking", ImpactBonus: 2},
	{KindPrefix: "property.type_changed", Severity: "breaking", ImpactBonus: 10},
	{KindPrefix: "field.became_required", Severity: "breaking", ImpactBonus: 8},
	{KindPrefix: "field.became_optional", Severity: "non_breaking", ImpactBonus: 2},

	// Enum rules
	{KindPrefix: "enum.value_removed", Severity: "breaking", ImpactBonus: 10},
	{KindPrefix: "enum.value_added", Severity: "non_breaking", ImpactBonus: 1},
	{KindPrefix: "enum.constraint_removed", Severity: "potential_breaking", ImpactBonus: 4},

	// Security rules
	{KindPrefix: "security.removed", Severity: "breaking", ImpactBonus: 10},
	{KindPrefix: "security.added", Severity: "potential_breaking", ImpactBonus: 5},

	// Proto service/method rules
	{KindPrefix: "service.removed", Severity: "breaking", ImpactBonus: 15},
	{KindPrefix: "service.added", Severity: "non_breaking", ImpactBonus: 1},
	{KindPrefix: "method.removed", Severity: "breaking", ImpactBonus: 10},
	{KindPrefix: "method.added", Severity: "non_breaking", ImpactBonus: 1},

	// Proto field rules — field number reuse is the most critical proto breaking change
	{KindPrefix: "proto.field_number_reused", Severity: "breaking", ImpactBonus: 20},
	{KindPrefix: "proto.field_removed", Severity: "breaking", ImpactBonus: 12},
	{KindPrefix: "proto.field_type_changed", Severity: "breaking", ImpactBonus: 12},

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
