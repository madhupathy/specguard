package change_classifier

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/specguard/specguard/internal/database"
	"github.com/specguard/specguard/internal/spec_diff"
)

type ClassificationRule struct {
	Pattern        string
	Classification spec_diff.Classification
	ChangeType     spec_diff.ChangeType
	Description    string
	ImpactBonus    int
}

type Classifier struct {
	db    *database.Database
	rules []ClassificationRule
}

func New(db *database.Database) *Classifier {
	c := &Classifier{db: db}
	c.initializeRules()
	return c
}

func (c *Classifier) initializeRules() {
	c.rules = []ClassificationRule{
		// Endpoint rules
		{
			Pattern:        "paths.*.delete",
			Classification: spec_diff.ClassificationEndpointRemoved,
			ChangeType:     spec_diff.ChangeTypeBreaking,
			Description:    "DELETE endpoint removed",
			ImpactBonus:    5,
		},
		{
			Pattern:        "paths.*.post",
			Classification: spec_diff.ClassificationEndpointAdded,
			ChangeType:     spec_diff.ChangeTypeAddition,
			Description:    "POST endpoint added",
			ImpactBonus:    1,
		},
		{
			Pattern:        "paths.*.get",
			Classification: spec_diff.ClassificationEndpointAdded,
			ChangeType:     spec_diff.ChangeTypeAddition,
			Description:    "GET endpoint added",
			ImpactBonus:    1,
		},

		// Schema rules
		{
			Pattern:        "components.schemas.*.required.*",
			Classification: spec_diff.ClassificationSchemaModified,
			ChangeType:     spec_diff.ChangeTypeBreaking,
			Description:    "Field became required",
			ImpactBonus:    3,
		},
		{
			Pattern:        "components.schemas.*.properties.*.type",
			Classification: spec_diff.ClassificationSchemaModified,
			ChangeType:     spec_diff.ChangeTypeBreaking,
			Description:    "Field type changed",
			ImpactBonus:    4,
		},
		{
			Pattern:        "components.schemas.*.properties.*.format",
			Classification: spec_diff.ClassificationSchemaModified,
			ChangeType:     spec_diff.ChangeTypeNonBreaking,
			Description:    "Field format changed",
			ImpactBonus:    1,
		},

		// Parameter rules
		{
			Pattern:        "parameters.*.required.*",
			Classification: spec_diff.ClassificationParameterRemoved,
			ChangeType:     spec_diff.ChangeTypeBreaking,
			Description:    "Required parameter removed",
			ImpactBonus:    3,
		},
		{
			Pattern:        "parameters.*.in",
			Classification: spec_diff.ClassificationParameterAdded,
			ChangeType:     spec_diff.ChangeTypeNonBreaking,
			Description:    "Parameter location changed",
			ImpactBonus:    2,
		},

		// Response rules
		{
			Pattern:        "responses.*.200",
			Classification: spec_diff.ClassificationResponseRemoved,
			ChangeType:     spec_diff.ChangeTypeBreaking,
			Description:    "Success response removed",
			ImpactBonus:    4,
		},
		{
			Pattern:        "responses.*.default",
			Classification: spec_diff.ClassificationResponseAdded,
			ChangeType:     spec_diff.ChangeTypeNonBreaking,
			Description:    "Default response added",
			ImpactBonus:    1,
		},

		// Security rules
		{
			Pattern:        "security.*",
			Classification: spec_diff.ClassificationSecurityAdded,
			ChangeType:     spec_diff.ChangeTypeBreaking,
			Description:    "Security requirement added",
			ImpactBonus:    3,
		},
	}
}

// ClassifyChange classifies a single change based on rules
func (c *Classifier) ClassifyChange(change *spec_diff.Change) *spec_diff.Change {
	// Find matching rule
	for _, rule := range c.rules {
		if c.matchesPattern(change.Path, rule.Pattern) {
			change.Classification = rule.Classification
			change.ChangeType = rule.ChangeType
			change.Description = rule.Description
			change.ImpactScore += rule.ImpactBonus

			// Store rule metadata
			if change.Metadata == nil {
				change.Metadata = make(map[string]interface{})
			}
			change.Metadata["rule_pattern"] = rule.Pattern
			change.Metadata["rule_description"] = rule.Description
			break
		}
	}

	// Enhanced classification based on path analysis
	c.enhanceClassification(change)

	return change
}

// ClassifyChanges classifies multiple changes
func (c *Classifier) ClassifyChanges(changes []spec_diff.Change) []spec_diff.Change {
	var classified []spec_diff.Change

	for i := range changes {
		classified = append(classified, *c.ClassifyChange(&changes[i]))
	}

	return classified
}

// GetClassificationSummary provides a summary of classifications
func (c *Classifier) GetClassificationSummary(changes []spec_diff.Change) map[string]int {
	summary := make(map[string]int)

	for _, change := range changes {
		key := string(change.Classification)
		summary[key]++
	}

	return summary
}

// GetHighImpactChanges returns changes with high impact scores
func (c *Classifier) GetHighImpactChanges(changes []spec_diff.Change, threshold int) []spec_diff.Change {
	var highImpact []spec_diff.Change

	for _, change := range changes {
		if change.ImpactScore >= threshold {
			highImpact = append(highImpact, change)
		}
	}

	return highImpact
}

// SaveClassification saves classification results to database
func (c *Classifier) SaveClassification(ctx context.Context, change *spec_diff.Change) error {
	query := `
		UPDATE changes
		SET change_type = $1, classification = $2, description = $3, impact_score = $4, metadata = $5
		WHERE id = $6
	`

	metadataBytes, _ := json.Marshal(change.Metadata)

	_, err := c.db.GetPool().Exec(ctx, query,
		change.ChangeType, change.Classification, change.Description,
		change.ImpactScore, metadataBytes, change.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to save classification: %w", err)
	}

	return nil
}

// Private helper methods

func (c *Classifier) matchesPattern(path, pattern string) bool {
	// Simple pattern matching - in production, use regex or more sophisticated matching
	patternParts := strings.Split(pattern, ".")
	pathParts := strings.Split(path, ".")

	if len(patternParts) != len(pathParts) {
		return false
	}

	for i, patternPart := range patternParts {
		if patternPart != "*" && patternPart != pathParts[i] {
			return false
		}
	}

	return true
}

func (c *Classifier) enhanceClassification(change *spec_diff.Change) {
	// Additional logic based on path analysis

	// If path contains "deprecated", mark as deprecation
	if strings.Contains(strings.ToLower(change.Path), "deprecated") {
		change.ChangeType = spec_diff.ChangeTypeDeprecation
		change.Description = "Deprecated functionality"
	}

	// If path contains "authentication" or "security", prioritize
	if strings.Contains(strings.ToLower(change.Path), "security") ||
		strings.Contains(strings.ToLower(change.Path), "auth") {
		change.ImpactScore += 2
		if change.Metadata == nil {
			change.Metadata = make(map[string]interface{})
		}
		change.Metadata["security_related"] = true
	}

	// If path contains "response" and it's a breaking change, increase impact
	if strings.Contains(strings.ToLower(change.Path), "response") &&
		change.ChangeType == spec_diff.ChangeTypeBreaking {
		change.ImpactScore += 2
	}

	// Add context-specific classifications
	if strings.HasPrefix(change.Path, "paths.") {
		if strings.Contains(change.Path, "parameters") {
			if change.Metadata == nil {
				change.Metadata = make(map[string]interface{})
			}
			change.Metadata["area"] = "parameters"
		} else if strings.Contains(change.Path, "responses") {
			if change.Metadata == nil {
				change.Metadata = make(map[string]interface{})
			}
			change.Metadata["area"] = "responses"
		}
	}
}
