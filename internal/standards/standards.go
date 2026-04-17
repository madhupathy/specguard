package standards

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Violation represents a single standards rule violation.
type Violation struct {
	RuleID      string `json:"rule_id"`
	RuleName    string `json:"rule_name"`
	Severity    string `json:"severity"`
	Path        string `json:"path"`
	Description string `json:"description"`
	Remediation string `json:"remediation"`
}

// Report captures the full standards analysis output.
type Report struct {
	GeneratedAt    string      `json:"generated_at"`
	TotalChecked   int         `json:"total_checked"`
	TotalViolation int         `json:"total_violations"`
	Violations     []Violation `json:"violations"`
}

// Analyze runs all 10 standards rules against a normalized OpenAPI spec.
// specPath points to the openapi.normalized.json file.
func Analyze(specPath string) (Report, error) {
	report := Report{Violations: []Violation{}}

	raw, err := os.ReadFile(specPath)
	if err != nil {
		return report, fmt.Errorf("read spec: %w", err)
	}

	var spec map[string]interface{}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return report, fmt.Errorf("parse spec: %w", err)
	}

	rules := []func(map[string]interface{}) []Violation{
		ruleConsistentErrorShape,
		rulePaginationConsistency,
		ruleVersioningPresent,
		ruleOperationIDPresent,
		ruleExamplesPresent,
		ruleAuthDocumented,
		ruleFieldNamingConvention,
		ruleNullableOptionalDiscipline,
		ruleHTTPStatusCoverage,
		ruleDeprecationMarkers,
	}

	for _, rule := range rules {
		report.TotalChecked++
		violations := rule(spec)
		report.Violations = append(report.Violations, violations...)
	}

	report.TotalViolation = len(report.Violations)
	return report, nil
}

// WriteReport writes the standards report as Markdown.
func WriteReport(report Report, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create reports dir: %w", err)
	}

	md := renderMarkdown(report)
	return os.WriteFile(filepath.Join(outDir, "standards.md"), []byte(md), 0o644)
}

func renderMarkdown(report Report) string {
	var b strings.Builder
	b.WriteString("# SpecGuard Standards Report\n\n")
	b.WriteString(fmt.Sprintf("Rules checked: %d\n", report.TotalChecked))
	b.WriteString(fmt.Sprintf("Violations found: **%d**\n\n", report.TotalViolation))

	if len(report.Violations) == 0 {
		b.WriteString("No violations detected.\n")
		return b.String()
	}

	b.WriteString("| # | Rule | Severity | Path | Description | Remediation |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for i, v := range report.Violations {
		b.WriteString(fmt.Sprintf("| %d | %s | %s | `%s` | %s | %s |\n",
			i+1, v.RuleName, v.Severity, v.Path, v.Description, v.Remediation))
	}

	return b.String()
}

// --- Rule 1: Consistent error shape ---
func ruleConsistentErrorShape(spec map[string]interface{}) []Violation {
	var violations []Violation
	paths := getMap(spec, "paths")
	for path, pathVal := range paths {
		pathInfo := asMap(pathVal)
		if pathInfo == nil {
			continue
		}
		for _, method := range httpMethods {
			methodInfo := asMap(pathInfo[method])
			if methodInfo == nil {
				continue
			}
			responses := getMap(methodInfo, "responses")
			errorCodes := []string{"400", "401", "403", "404", "500", "default"}
			hasAnyError := false
			for _, code := range errorCodes {
				if _, ok := responses[code]; ok {
					hasAnyError = true
					break
				}
			}
			if !hasAnyError && len(responses) > 0 {
				violations = append(violations, Violation{
					RuleID:      "STD-001",
					RuleName:    "Consistent error shape",
					Severity:    "medium",
					Path:        fmt.Sprintf("%s %s", strings.ToUpper(method), path),
					Description: "No error responses (4xx/5xx/default) documented",
					Remediation: "Add at least a default or 4xx error response with a consistent error schema.",
				})
			}
		}
	}
	return violations
}

// --- Rule 2: Pagination consistency ---
func rulePaginationConsistency(spec map[string]interface{}) []Violation {
	var violations []Violation
	paths := getMap(spec, "paths")
	paginationStyles := map[string]int{}

	for path, pathVal := range paths {
		pathInfo := asMap(pathVal)
		if pathInfo == nil {
			continue
		}
		getMethod := asMap(pathInfo["get"])
		if getMethod == nil {
			continue
		}
		if !looksLikeListEndpoint(path) {
			continue
		}
		params := getParamNames(getMethod)
		style := detectPaginationStyle(params)
		if style != "" {
			paginationStyles[style]++
		}
	}

	if len(paginationStyles) > 1 {
		styles := []string{}
		for s := range paginationStyles {
			styles = append(styles, s)
		}
		violations = append(violations, Violation{
			RuleID:      "STD-002",
			RuleName:    "Pagination consistency",
			Severity:    "medium",
			Path:        "(global)",
			Description: fmt.Sprintf("Multiple pagination styles detected: %s", strings.Join(styles, ", ")),
			Remediation: "Standardize on a single pagination approach (offset/limit, cursor, or page/pageSize).",
		})
	}
	return violations
}

// --- Rule 3: Versioning present ---
func ruleVersioningPresent(spec map[string]interface{}) []Violation {
	var violations []Violation

	info := getMap(spec, "info")
	version, _ := info["version"].(string)
	if version == "" {
		violations = append(violations, Violation{
			RuleID:      "STD-003",
			RuleName:    "Versioning present",
			Severity:    "high",
			Path:        "info.version",
			Description: "API version not specified in info.version",
			Remediation: "Set info.version to a semantic version string (e.g., 1.0.0).",
		})
	}

	paths := getMap(spec, "paths")
	hasVersionedPath := false
	for path := range paths {
		if strings.Contains(path, "/v1") || strings.Contains(path, "/v2") || strings.Contains(path, "/v3") {
			hasVersionedPath = true
			break
		}
	}

	servers := getSlice(spec, "servers")
	hasVersionedServer := false
	for _, s := range servers {
		sm := asMap(s)
		if sm == nil {
			continue
		}
		url, _ := sm["url"].(string)
		if strings.Contains(url, "/v1") || strings.Contains(url, "/v2") || strings.Contains(url, "/v3") {
			hasVersionedServer = true
			break
		}
	}

	if !hasVersionedPath && !hasVersionedServer && version == "" {
		violations = append(violations, Violation{
			RuleID:      "STD-003",
			RuleName:    "Versioning present",
			Severity:    "medium",
			Path:        "(global)",
			Description: "No versioning detected in paths, servers, or info",
			Remediation: "Add version prefix to paths (e.g., /v1/) or use server URL versioning.",
		})
	}

	return violations
}

// --- Rule 4: operationId present ---
func ruleOperationIDPresent(spec map[string]interface{}) []Violation {
	var violations []Violation
	paths := getMap(spec, "paths")
	for path, pathVal := range paths {
		pathInfo := asMap(pathVal)
		if pathInfo == nil {
			continue
		}
		for _, method := range httpMethods {
			methodInfo := asMap(pathInfo[method])
			if methodInfo == nil {
				continue
			}
			opID, _ := methodInfo["operationId"].(string)
			if opID == "" {
				violations = append(violations, Violation{
					RuleID:      "STD-004",
					RuleName:    "operationId present",
					Severity:    "low",
					Path:        fmt.Sprintf("%s %s", strings.ToUpper(method), path),
					Description: "Missing operationId",
					Remediation: "Add a unique operationId for SDK generation and documentation linking.",
				})
			}
		}
	}
	return violations
}

// --- Rule 5: Request + response examples ---
func ruleExamplesPresent(spec map[string]interface{}) []Violation {
	var violations []Violation
	paths := getMap(spec, "paths")
	for path, pathVal := range paths {
		pathInfo := asMap(pathVal)
		if pathInfo == nil {
			continue
		}
		for _, method := range httpMethods {
			methodInfo := asMap(pathInfo[method])
			if methodInfo == nil {
				continue
			}
			responses := getMap(methodInfo, "responses")
			for code, respVal := range responses {
				resp := asMap(respVal)
				if resp == nil {
					continue
				}
				content := getMap(resp, "content")
				if len(content) == 0 {
					continue
				}
				hasExample := false
				for _, mediaVal := range content {
					media := asMap(mediaVal)
					if media == nil {
						continue
					}
					if _, ok := media["example"]; ok {
						hasExample = true
					}
					if _, ok := media["examples"]; ok {
						hasExample = true
					}
					schema := asMap(media["schema"])
					if schema != nil {
						if _, ok := schema["example"]; ok {
							hasExample = true
						}
					}
				}
				if !hasExample {
					violations = append(violations, Violation{
						RuleID:      "STD-005",
						RuleName:    "Examples present",
						Severity:    "low",
						Path:        fmt.Sprintf("%s %s responses.%s", strings.ToUpper(method), path, code),
						Description: "Response missing example",
						Remediation: "Add example or examples to response media type for documentation quality.",
					})
				}
			}
		}
	}
	return violations
}

// --- Rule 6: Auth documented ---
func ruleAuthDocumented(spec map[string]interface{}) []Violation {
	var violations []Violation

	securitySchemes := getNestedMap(spec, "components", "securitySchemes")
	globalSecurity := getSlice(spec, "security")

	if len(securitySchemes) == 0 && len(globalSecurity) == 0 {
		violations = append(violations, Violation{
			RuleID:      "STD-006",
			RuleName:    "Auth documented",
			Severity:    "high",
			Path:        "(global)",
			Description: "No security schemes or global security requirements defined",
			Remediation: "Add components.securitySchemes and a global security requirement.",
		})
	}

	return violations
}

// --- Rule 7: Field naming convention ---
func ruleFieldNamingConvention(spec map[string]interface{}) []Violation {
	var violations []Violation
	schemas := getNestedMap(spec, "components", "schemas")

	snakeCount := 0
	camelCount := 0
	total := 0

	for _, schemaVal := range schemas {
		schema := asMap(schemaVal)
		if schema == nil {
			continue
		}
		props := getMap(schema, "properties")
		for propName := range props {
			total++
			if strings.Contains(propName, "_") {
				snakeCount++
			} else if len(propName) > 0 && propName[0] >= 'a' && propName[0] <= 'z' && strings.ContainsAny(propName, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
				camelCount++
			}
		}
	}

	if snakeCount > 0 && camelCount > 0 && total > 5 {
		violations = append(violations, Violation{
			RuleID:      "STD-007",
			RuleName:    "Field naming convention",
			Severity:    "low",
			Path:        "components.schemas",
			Description: fmt.Sprintf("Mixed naming conventions: %d snake_case, %d camelCase out of %d fields", snakeCount, camelCount, total),
			Remediation: "Standardize on either snake_case or camelCase for all schema properties.",
		})
	}

	return violations
}

// --- Rule 8: Nullable/optional discipline ---
func ruleNullableOptionalDiscipline(spec map[string]interface{}) []Violation {
	var violations []Violation
	schemas := getNestedMap(spec, "components", "schemas")

	for schemaName, schemaVal := range schemas {
		schema := asMap(schemaVal)
		if schema == nil {
			continue
		}
		required := getStringSlice(schema, "required")
		requiredSet := toSet(required)
		props := getMap(schema, "properties")

		for propName, propVal := range props {
			prop := asMap(propVal)
			if prop == nil {
				continue
			}
			nullable, hasNullable := prop["nullable"]
			_, isRequired := requiredSet[propName]

			if hasNullable && nullable == true && isRequired {
				violations = append(violations, Violation{
					RuleID:      "STD-008",
					RuleName:    "Nullable/optional discipline",
					Severity:    "medium",
					Path:        fmt.Sprintf("components.schemas.%s.properties.%s", schemaName, propName),
					Description: "Field is both required and nullable — ambiguous contract",
					Remediation: "Decide if the field is truly required (non-null) or optional (nullable).",
				})
			}
		}
	}

	return violations
}

// --- Rule 9: HTTP status coverage ---
func ruleHTTPStatusCoverage(spec map[string]interface{}) []Violation {
	var violations []Violation
	paths := getMap(spec, "paths")
	for path, pathVal := range paths {
		pathInfo := asMap(pathVal)
		if pathInfo == nil {
			continue
		}
		for _, method := range httpMethods {
			methodInfo := asMap(pathInfo[method])
			if methodInfo == nil {
				continue
			}
			responses := getMap(methodInfo, "responses")
			if len(responses) == 0 {
				violations = append(violations, Violation{
					RuleID:      "STD-009",
					RuleName:    "HTTP status coverage",
					Severity:    "medium",
					Path:        fmt.Sprintf("%s %s", strings.ToUpper(method), path),
					Description: "No responses documented",
					Remediation: "Document at least success (2xx) and common error (4xx) responses.",
				})
				continue
			}
			hasSuccess := false
			for code := range responses {
				if strings.HasPrefix(code, "2") || code == "default" {
					hasSuccess = true
					break
				}
			}
			if !hasSuccess {
				violations = append(violations, Violation{
					RuleID:      "STD-009",
					RuleName:    "HTTP status coverage",
					Severity:    "medium",
					Path:        fmt.Sprintf("%s %s", strings.ToUpper(method), path),
					Description: "No success response (2xx) documented",
					Remediation: "Add at least a 200/201/204 response.",
				})
			}
		}
	}
	return violations
}

// --- Rule 10: Deprecation markers ---
func ruleDeprecationMarkers(spec map[string]interface{}) []Violation {
	var violations []Violation
	paths := getMap(spec, "paths")
	for path, pathVal := range paths {
		pathInfo := asMap(pathVal)
		if pathInfo == nil {
			continue
		}
		for _, method := range httpMethods {
			methodInfo := asMap(pathInfo[method])
			if methodInfo == nil {
				continue
			}
			deprecated, _ := methodInfo["deprecated"].(bool)
			if deprecated {
				desc, _ := methodInfo["description"].(string)
				summary, _ := methodInfo["summary"].(string)
				combined := strings.ToLower(desc + " " + summary)
				if !strings.Contains(combined, "sunset") && !strings.Contains(combined, "removal") && !strings.Contains(combined, "replaced") && !strings.Contains(combined, "migrate") {
					violations = append(violations, Violation{
						RuleID:      "STD-010",
						RuleName:    "Deprecation markers",
						Severity:    "low",
						Path:        fmt.Sprintf("%s %s", strings.ToUpper(method), path),
						Description: "Endpoint marked deprecated but no sunset/migration guidance in description",
						Remediation: "Add sunset date or migration instructions to the description.",
					})
				}
			}
		}
	}
	return violations
}

// --- Helpers ---

var httpMethods = []string{"get", "post", "put", "delete", "patch", "head", "options"}

func asMap(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return nil
}

func getMap(data map[string]interface{}, key string) map[string]interface{} {
	if v, ok := data[key]; ok {
		if m := asMap(v); m != nil {
			return m
		}
	}
	return map[string]interface{}{}
}

func getNestedMap(data map[string]interface{}, keys ...string) map[string]interface{} {
	current := data
	for _, key := range keys {
		next := asMap(current[key])
		if next == nil {
			return map[string]interface{}{}
		}
		current = next
	}
	return current
}

func getSlice(data map[string]interface{}, key string) []interface{} {
	if v, ok := data[key]; ok {
		if s, ok := v.([]interface{}); ok {
			return s
		}
	}
	return nil
}

func getStringSlice(data map[string]interface{}, key string) []string {
	raw := getSlice(data, key)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}

func getParamNames(method map[string]interface{}) map[string]bool {
	params := map[string]bool{}
	for _, p := range getSlice(method, "parameters") {
		pm := asMap(p)
		if pm == nil {
			continue
		}
		if name, ok := pm["name"].(string); ok {
			params[name] = true
		}
	}
	return params
}

func looksLikeListEndpoint(path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return false
	}
	last := parts[len(parts)-1]
	if strings.HasPrefix(last, "{") {
		return false
	}
	return true
}

func detectPaginationStyle(params map[string]bool) string {
	if params["cursor"] || params["after"] || params["before"] {
		return "cursor"
	}
	if params["offset"] && params["limit"] {
		return "offset/limit"
	}
	if params["page"] || params["pageSize"] || params["page_size"] {
		return "page/pageSize"
	}
	if params["limit"] {
		return "limit"
	}
	return ""
}
