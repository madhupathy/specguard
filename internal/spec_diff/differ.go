package spec_diff

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/specguard/specguard/internal/database"
	"github.com/specguard/specguard/internal/spec_loader"
)

type ChangeType string

const (
	ChangeTypeBreaking    ChangeType = "breaking"
	ChangeTypeNonBreaking ChangeType = "non_breaking"
	ChangeTypeDeprecation ChangeType = "deprecation"
	ChangeTypeAddition    ChangeType = "addition"
)

type Classification string

const (
	ClassificationEndpointAdded    Classification = "endpoint_added"
	ClassificationEndpointRemoved  Classification = "endpoint_removed"
	ClassificationEndpointModified Classification = "endpoint_modified"
	ClassificationSchemaAdded      Classification = "schema_added"
	ClassificationSchemaRemoved    Classification = "schema_removed"
	ClassificationSchemaModified   Classification = "schema_modified"
	ClassificationParameterAdded   Classification = "parameter_added"
	ClassificationParameterRemoved Classification = "parameter_removed"
	ClassificationResponseAdded    Classification = "response_added"
	ClassificationResponseRemoved  Classification = "response_removed"
	ClassificationSecurityAdded    Classification = "security_added"
	ClassificationSecurityRemoved  Classification = "security_removed"
)

type Change struct {
	ID             int64                  `json:"id"`
	FromSpecID     int64                  `json:"from_spec_id"`
	ToSpecID       int64                  `json:"to_spec_id"`
	ChangeType     ChangeType             `json:"change_type"`
	Classification Classification         `json:"classification"`
	Path           string                 `json:"path"`
	Description    string                 `json:"description"`
	AISummary      string                 `json:"ai_summary,omitempty"`
	ImpactScore    int                    `json:"impact_score"`
	Metadata       map[string]interface{} `json:"metadata"`
}

type DiffResult struct {
	Changes []Change `json:"changes"`
	Summary struct {
		Total        int `json:"total"`
		Breaking     int `json:"breaking"`
		NonBreaking  int `json:"non_breaking"`
		Deprecations int `json:"deprecations"`
		Additions    int `json:"additions"`
	} `json:"summary"`
}

type Differ struct {
	db *database.Database
}

func New(db *database.Database) *Differ {
	return &Differ{db: db}
}

// Compare compares two specs and returns the differences
func (d *Differ) Compare(ctx context.Context, fromSpec, toSpec *spec_loader.Spec) (*DiffResult, error) {
	result := &DiffResult{}

	// Compare the content
	changes := d.compareContent(fromSpec.Content, toSpec.Content)

	// Process each change
	for _, change := range changes {
		change.FromSpecID = fromSpec.ID
		change.ToSpecID = toSpec.ID

		// Calculate impact score
		change.ImpactScore = d.calculateImpactScore(change)

		result.Changes = append(result.Changes, change)

		// Update summary
		switch change.ChangeType {
		case ChangeTypeBreaking:
			result.Summary.Breaking++
		case ChangeTypeNonBreaking:
			result.Summary.NonBreaking++
		case ChangeTypeDeprecation:
			result.Summary.Deprecations++
		case ChangeTypeAddition:
			result.Summary.Additions++
		}
		result.Summary.Total++
	}

	return result, nil
}

// SaveChanges saves the diff results to the database
func (d *Differ) SaveChanges(ctx context.Context, result *DiffResult) error {
	for _, change := range result.Changes {
		query := `
			INSERT INTO changes (from_spec_id, to_spec_id, change_type, classification, path, description, ai_summary, impact_score, metadata)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id
		`

		metadataBytes, _ := json.Marshal(change.Metadata)

		var id int64
		err := d.db.GetPool().QueryRow(ctx, query,
			change.FromSpecID, change.ToSpecID, change.ChangeType, change.Classification,
			change.Path, change.Description, change.AISummary, change.ImpactScore, metadataBytes,
		).Scan(&id)

		if err != nil {
			return fmt.Errorf("failed to save change: %w", err)
		}

		change.ID = id
	}

	return nil
}

// GetChanges retrieves changes for a spec
func (d *Differ) GetChanges(ctx context.Context, specID int64) ([]Change, error) {
	query := `
		SELECT id, from_spec_id, to_spec_id, change_type, classification, path, description, ai_summary, impact_score, metadata
		FROM changes
		WHERE to_spec_id = $1
		ORDER BY impact_score DESC, created_at ASC
	`

	rows, err := d.db.GetPool().Query(ctx, query, specID)
	if err != nil {
		return nil, fmt.Errorf("failed to get changes: %w", err)
	}
	defer rows.Close()

	var changes []Change
	for rows.Next() {
		var change Change
		var metadataBytes []byte

		err := rows.Scan(
			&change.ID, &change.FromSpecID, &change.ToSpecID, &change.ChangeType,
			&change.Classification, &change.Path, &change.Description, &change.AISummary,
			&change.ImpactScore, &metadataBytes,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan change: %w", err)
		}

		if metadataBytes != nil {
			if err := json.Unmarshal(metadataBytes, &change.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		changes = append(changes, change)
	}

	return changes, nil
}

// Core comparison logic

func (d *Differ) compareContent(from, to map[string]interface{}) []Change {
	var changes []Change

	// Compare OpenAPI paths
	if _, ok := to["paths"].(map[string]interface{}); ok {
		changes = append(changes, d.comparePaths(from, to)...)
	}

	// Compare components/schemas
	if components, ok := to["components"].(map[string]interface{}); ok {
		if _, ok := components["schemas"].(map[string]interface{}); ok {
			changes = append(changes, d.compareSchemas(from, to)...)
		}
	}

	return changes
}

func (d *Differ) comparePaths(from, to map[string]interface{}) []Change {
	var changes []Change

	fromPaths := getPaths(from)
	toPaths := getPaths(to)

	// Check for removed endpoints
	for path := range fromPaths {
		fromInfo, _ := fromPaths[path].(map[string]interface{})
		if _, exists := toPaths[path]; !exists {
			changes = append(changes, Change{
				ChangeType:     ChangeTypeBreaking,
				Classification: ClassificationEndpointRemoved,
				Path:           path,
				Description:    fmt.Sprintf("Endpoint %s has been removed", path),
				Metadata: map[string]interface{}{
					"old_methods": getMethods(fromInfo),
				},
			})
		}
	}

	// Check for added and modified endpoints
	for path, toPathVal := range toPaths {
		toPathInfo, _ := toPathVal.(map[string]interface{})
		if toPathInfo == nil {
			continue
		}
		if fromPathInfo, exists := fromPaths[path]; !exists {
			// New endpoint
			changes = append(changes, Change{
				ChangeType:     ChangeTypeAddition,
				Classification: ClassificationEndpointAdded,
				Path:           path,
				Description:    fmt.Sprintf("New endpoint %s added", path),
				Metadata: map[string]interface{}{
					"new_methods": getMethods(toPathInfo),
				},
			})
		} else {
			fromPathInfoMap, _ := fromPathInfo.(map[string]interface{})
			if fromPathInfoMap == nil {
				continue
			}
			// Modified endpoint
			pathChanges := d.compareEndpoint(path, fromPathInfoMap, toPathInfo)
			changes = append(changes, pathChanges...)
		}
	}

	return changes
}

func (d *Differ) compareSchemas(from, to map[string]interface{}) []Change {
	var changes []Change

	fromSchemas := getSchemas(from)
	toSchemas := getSchemas(to)

	// Check for removed schemas
	for schemaName := range fromSchemas {
		if _, exists := toSchemas[schemaName]; !exists {
			changes = append(changes, Change{
				ChangeType:     ChangeTypeBreaking,
				Classification: ClassificationSchemaRemoved,
				Path:           fmt.Sprintf("components.schemas.%s", schemaName),
				Description:    fmt.Sprintf("Schema %s has been removed", schemaName),
			})
		}
	}

	// Check for added and modified schemas
	for schemaName, toSchemaVal := range toSchemas {
		toSchema, _ := toSchemaVal.(map[string]interface{})
		if toSchema == nil {
			continue
		}
		if fromSchema, exists := fromSchemas[schemaName]; !exists {
			// New schema
			changes = append(changes, Change{
				ChangeType:     ChangeTypeAddition,
				Classification: ClassificationSchemaAdded,
				Path:           fmt.Sprintf("components.schemas.%s", schemaName),
				Description:    fmt.Sprintf("New schema %s added", schemaName),
			})
		} else {
			fromSchemaMap, _ := fromSchema.(map[string]interface{})
			if fromSchemaMap == nil {
				continue
			}
			// Modified schema
			schemaChanges := d.compareSchema(schemaName, fromSchemaMap, toSchema)
			changes = append(changes, schemaChanges...)
		}
	}

	return changes
}

func (d *Differ) compareEndpoint(path string, from, to map[string]interface{}) []Change {
	var changes []Change

	fromMethods := getMethods(from)
	toMethods := getMethods(to)

	// Check for removed methods
	for method := range fromMethods {
		if _, exists := toMethods[method]; !exists {
			changes = append(changes, Change{
				ChangeType:     ChangeTypeBreaking,
				Classification: ClassificationEndpointRemoved,
				Path:           fmt.Sprintf("%s %s", strings.ToUpper(method), path),
				Description:    fmt.Sprintf("Method %s %s has been removed", strings.ToUpper(method), path),
			})
		}
	}

	// Check for added and modified methods
	for method, toMethodVal := range toMethods {
		toMethodInfo, _ := toMethodVal.(map[string]interface{})
		if toMethodInfo == nil {
			continue
		}
		if fromMethodInfo, exists := fromMethods[method]; !exists {
			// New method
			changes = append(changes, Change{
				ChangeType:     ChangeTypeAddition,
				Classification: ClassificationEndpointAdded,
				Path:           fmt.Sprintf("%s %s", strings.ToUpper(method), path),
				Description:    fmt.Sprintf("New method %s %s added", strings.ToUpper(method), path),
			})
		} else {
			fromMethodMap, _ := fromMethodInfo.(map[string]interface{})
			if fromMethodMap == nil {
				continue
			}
			// Modified method
			methodChanges := d.compareMethod(path, method, fromMethodMap, toMethodInfo)
			changes = append(changes, methodChanges...)
		}
	}

	return changes
}

func (d *Differ) compareMethod(path, method string, from, to map[string]interface{}) []Change {
	var changes []Change

	// Compare parameters
	fromParams := getParameters(from)
	toParams := getParameters(to)

	// Check for removed parameters
	for paramName := range fromParams {
		if _, exists := toParams[paramName]; !exists {
			changes = append(changes, Change{
				ChangeType:     ChangeTypeBreaking,
				Classification: ClassificationParameterRemoved,
				Path:           fmt.Sprintf("%s %s.parameters.%s", strings.ToUpper(method), path, paramName),
				Description:    fmt.Sprintf("Parameter %s removed from %s %s", paramName, strings.ToUpper(method), path),
			})
		}
	}

	// Check for added parameters
	for paramName := range toParams {
		if _, exists := fromParams[paramName]; !exists {
			changes = append(changes, Change{
				ChangeType:     ChangeTypeNonBreaking,
				Classification: ClassificationParameterAdded,
				Path:           fmt.Sprintf("%s %s.parameters.%s", strings.ToUpper(method), path, paramName),
				Description:    fmt.Sprintf("New parameter %s added to %s %s", paramName, strings.ToUpper(method), path),
			})
		}
	}

	return changes
}

func (d *Differ) compareSchema(schemaName string, from, to map[string]interface{}) []Change {
	var changes []Change

	// Compare required fields
	fromRequired := getRequiredFields(from)
	toRequired := getRequiredFields(to)

	// Check for newly required fields (breaking change)
	for _, field := range toRequired {
		if !contains(fromRequired, field) {
			changes = append(changes, Change{
				ChangeType:     ChangeTypeBreaking,
				Classification: ClassificationSchemaModified,
				Path:           fmt.Sprintf("components.schemas.%s.required.%s", schemaName, field),
				Description:    fmt.Sprintf("Field %s in schema %s is now required", field, schemaName),
			})
		}
	}

	return changes
}

func (d *Differ) calculateImpactScore(change Change) int {
	score := 0

	switch change.ChangeType {
	case ChangeTypeBreaking:
		score = 10
	case ChangeTypeDeprecation:
		score = 5
	case ChangeTypeNonBreaking:
		score = 2
	case ChangeTypeAddition:
		score = 1
	}

	// Adjust based on classification
	switch change.Classification {
	case ClassificationEndpointRemoved:
		score += 5
	case ClassificationSchemaRemoved:
		score += 4
	case ClassificationParameterRemoved:
		score += 3
	}

	return score
}

// Helper functions

func getPaths(spec map[string]interface{}) map[string]interface{} {
	if paths, ok := spec["paths"].(map[string]interface{}); ok {
		return paths
	}
	return make(map[string]interface{})
}

func getSchemas(spec map[string]interface{}) map[string]interface{} {
	if components, ok := spec["components"].(map[string]interface{}); ok {
		if schemas, ok := components["schemas"].(map[string]interface{}); ok {
			return schemas
		}
	}
	return make(map[string]interface{})
}

func getMethods(pathInfo map[string]interface{}) map[string]interface{} {
	methods := make(map[string]interface{})
	for key, value := range pathInfo {
		if isHTTPMethod(key) {
			methods[key] = value
		}
	}
	return methods
}

func getParameters(methodInfo map[string]interface{}) map[string]interface{} {
	if params, ok := methodInfo["parameters"].([]interface{}); ok {
		result := make(map[string]interface{})
		for _, param := range params {
			if paramMap, ok := param.(map[string]interface{}); ok {
				if name, ok := paramMap["name"].(string); ok {
					result[name] = param
				}
			}
		}
		return result
	}
	return make(map[string]interface{})
}

func getRequiredFields(schema map[string]interface{}) []string {
	if required, ok := schema["required"].([]interface{}); ok {
		var result []string
		for _, field := range required {
			if fieldStr, ok := field.(string); ok {
				result = append(result, fieldStr)
			}
		}
		return result
	}
	return []string{}
}

func isHTTPMethod(method string) bool {
	methods := []string{"get", "post", "put", "delete", "patch", "head", "options"}
	for _, m := range methods {
		if strings.ToLower(method) == m {
			return true
		}
	}
	return false
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
