// Package protocol implements a Self-Optimizing API Fabric (SOAF)-inspired
// decision model that analyzes OpenAPI spec signals to produce per-endpoint
// REST vs gRPC recommendations.
package protocol

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Recommendation is the per-endpoint protocol verdict.
type Recommendation struct {
	Endpoint   string            `json:"endpoint"`
	Protocol   string            `json:"protocol"`   // "REST", "gRPC", "either"
	Confidence float64           `json:"confidence"` // 0-1
	Rationale  string            `json:"rationale"`
	Signals    map[string]string `json:"signals"`
}

// Report captures the full protocol analysis.
type Report struct {
	GeneratedAt     string           `json:"generated_at"`
	TotalEndpoints  int              `json:"total_endpoints"`
	RESTPreferred   int              `json:"rest_preferred"`
	GRPCPreferred   int              `json:"grpc_preferred"`
	Either          int              `json:"either"`
	HasProto        bool             `json:"has_proto"`
	Recommendations []Recommendation `json:"recommendations"`
}

// AnalyzeSpec reads an OpenAPI spec and optionally checks for protobuf presence
// to produce per-endpoint protocol recommendations.
func AnalyzeSpec(specPath string, hasProtobuf bool) (Report, error) {
	report := Report{
		HasProto:        hasProtobuf,
		Recommendations: []Recommendation{},
	}

	raw, err := os.ReadFile(specPath)
	if err != nil {
		return report, fmt.Errorf("read spec: %w", err)
	}
	var spec map[string]interface{}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return report, fmt.Errorf("parse spec: %w", err)
	}

	paths := getMap(spec, "paths")
	methods := []string{"get", "post", "put", "delete", "patch"}

	for path, pathVal := range paths {
		pathInfo := asMap(pathVal)
		if pathInfo == nil {
			continue
		}
		for _, method := range methods {
			opVal, ok := pathInfo[method]
			if !ok {
				continue
			}
			op := asMap(opVal)
			endpoint := fmt.Sprintf("%s %s", strings.ToUpper(method), path)
			rec := analyzeEndpoint(endpoint, method, path, op, hasProtobuf)
			report.Recommendations = append(report.Recommendations, rec)
		}
	}

	report.TotalEndpoints = len(report.Recommendations)
	for _, r := range report.Recommendations {
		switch r.Protocol {
		case "REST":
			report.RESTPreferred++
		case "gRPC":
			report.GRPCPreferred++
		default:
			report.Either++
		}
	}

	return report, nil
}

// analyzeEndpoint applies SOAF-inspired heuristics to a single endpoint.
func analyzeEndpoint(endpoint, method, path string, op map[string]interface{}, hasProto bool) Recommendation {
	signals := map[string]string{}
	var restScore, grpcScore float64

	// Signal 1: CRUD pattern — REST excels at resource-oriented CRUD
	if isCRUDPattern(method, path) {
		signals["crud_pattern"] = "true"
		restScore += 2.0
	} else {
		signals["crud_pattern"] = "false"
	}

	// Signal 2: Payload complexity — deep nesting favors gRPC (schema-first)
	schemaDepth := estimateResponseSchemaDepth(op)
	signals["schema_depth"] = fmt.Sprintf("%d", schemaDepth)
	if schemaDepth >= 3 {
		grpcScore += 1.5
	} else if schemaDepth >= 2 {
		grpcScore += 0.5
	}

	// Signal 3: Streaming hint — if description mentions streaming/events/realtime
	if hasStreamingHint(op) {
		signals["streaming_hint"] = "true"
		grpcScore += 3.0
	} else {
		signals["streaming_hint"] = "false"
	}

	// Signal 4: Batch/bulk operations — gRPC is more efficient for bulk
	if isBatchEndpoint(path, op) {
		signals["batch_operation"] = "true"
		grpcScore += 2.0
	} else {
		signals["batch_operation"] = "false"
	}

	// Signal 5: Binary/file transfer — REST is more natural
	if hasBinaryContent(op) {
		signals["binary_content"] = "true"
		restScore += 2.0
	} else {
		signals["binary_content"] = "false"
	}

	// Signal 6: Idempotency — PUT/DELETE are naturally idempotent in REST
	if method == "put" || method == "delete" {
		signals["idempotent"] = "true"
		restScore += 0.5
	}

	// Signal 7: Public API / browser-facing — REST preferred
	if isPublicFacing(op) {
		signals["public_facing"] = "true"
		restScore += 1.5
	} else {
		signals["public_facing"] = "false"
	}

	// Signal 8: High-frequency internal — gRPC preferred
	if isInternalService(path) {
		signals["internal_service"] = "true"
		grpcScore += 1.5
	} else {
		signals["internal_service"] = "false"
	}

	// Signal 9: Proto availability — can only recommend gRPC if proto exists
	if !hasProto {
		signals["proto_available"] = "false"
		grpcScore *= 0.3 // heavily penalize gRPC if no proto
	} else {
		signals["proto_available"] = "true"
	}

	// Decision
	rec := Recommendation{
		Endpoint: endpoint,
		Signals:  signals,
	}

	diff := restScore - grpcScore
	total := restScore + grpcScore
	if total == 0 {
		total = 1
	}

	switch {
	case diff > 1.5:
		rec.Protocol = "REST"
		rec.Confidence = clamp(0.5+diff/(2*total), 0.5, 0.95)
		rec.Rationale = buildRationale("REST", signals)
	case diff < -1.5:
		rec.Protocol = "gRPC"
		rec.Confidence = clamp(0.5+(-diff)/(2*total), 0.5, 0.95)
		rec.Rationale = buildRationale("gRPC", signals)
	default:
		rec.Protocol = "either"
		rec.Confidence = 0.5
		rec.Rationale = "No strong signal favoring either protocol; both are viable."
	}

	return rec
}

func isCRUDPattern(method, path string) bool {
	resourcePattern := strings.Count(path, "/") <= 3
	crudMethods := map[string]bool{"get": true, "post": true, "put": true, "delete": true, "patch": true}
	lower := strings.ToLower(path)
	nonCRUD := strings.Contains(lower, "batch") || strings.Contains(lower, "bulk") ||
		strings.Contains(lower, "stream") || strings.Contains(lower, "event") ||
		strings.Contains(lower, "webhook") || strings.Contains(lower, "rpc")
	return crudMethods[method] && resourcePattern && !nonCRUD
}

func estimateResponseSchemaDepth(op map[string]interface{}) int {
	responses := asMap(op["responses"])
	if responses == nil {
		return 0
	}
	maxDepth := 0
	for _, respVal := range responses {
		resp := asMap(respVal)
		if resp == nil {
			continue
		}
		content := asMap(resp["content"])
		if content == nil {
			continue
		}
		for _, mediaVal := range content {
			media := asMap(mediaVal)
			if media == nil {
				continue
			}
			schema := asMap(media["schema"])
			if schema != nil {
				d := schemaDepth(schema, 0)
				if d > maxDepth {
					maxDepth = d
				}
			}
		}
	}
	return maxDepth
}

func schemaDepth(schema map[string]interface{}, current int) int {
	if current > 10 {
		return current
	}
	max := current

	if props := asMap(schema["properties"]); props != nil {
		for _, propVal := range props {
			prop := asMap(propVal)
			if prop != nil {
				d := schemaDepth(prop, current+1)
				if d > max {
					max = d
				}
			}
		}
	}

	if items := asMap(schema["items"]); items != nil {
		d := schemaDepth(items, current+1)
		if d > max {
			max = d
		}
	}

	return max
}

func hasStreamingHint(op map[string]interface{}) bool {
	desc := strings.ToLower(getString(op, "description") + " " + getString(op, "summary"))
	keywords := []string{"stream", "realtime", "real-time", "event", "websocket", "sse", "push"}
	for _, kw := range keywords {
		if strings.Contains(desc, kw) {
			return true
		}
	}
	return false
}

func isBatchEndpoint(path string, op map[string]interface{}) bool {
	lower := strings.ToLower(path)
	if strings.Contains(lower, "batch") || strings.Contains(lower, "bulk") {
		return true
	}
	desc := strings.ToLower(getString(op, "description") + " " + getString(op, "summary"))
	return strings.Contains(desc, "batch") || strings.Contains(desc, "bulk")
}

func hasBinaryContent(op map[string]interface{}) bool {
	rb := asMap(op["requestBody"])
	if rb == nil {
		return false
	}
	content := asMap(rb["content"])
	if content == nil {
		return false
	}
	for mediaType := range content {
		if strings.Contains(mediaType, "octet-stream") || strings.Contains(mediaType, "multipart") {
			return true
		}
	}
	return false
}

func isPublicFacing(op map[string]interface{}) bool {
	tags := getStringSlice(op, "tags")
	for _, tag := range tags {
		lower := strings.ToLower(tag)
		if strings.Contains(lower, "public") || strings.Contains(lower, "external") || strings.Contains(lower, "portal") {
			return true
		}
	}
	return false
}

func isInternalService(path string) bool {
	lower := strings.ToLower(path)
	return strings.Contains(lower, "/internal/") || strings.Contains(lower, "/svc/") || strings.Contains(lower, "/rpc/")
}

func buildRationale(protocol string, signals map[string]string) string {
	var reasons []string
	if protocol == "REST" {
		if signals["crud_pattern"] == "true" {
			reasons = append(reasons, "resource-oriented CRUD pattern")
		}
		if signals["binary_content"] == "true" {
			reasons = append(reasons, "binary/file content transfer")
		}
		if signals["public_facing"] == "true" {
			reasons = append(reasons, "public/browser-facing API")
		}
		if signals["idempotent"] == "true" {
			reasons = append(reasons, "naturally idempotent operation")
		}
	} else {
		if signals["streaming_hint"] == "true" {
			reasons = append(reasons, "streaming/realtime requirements")
		}
		if signals["batch_operation"] == "true" {
			reasons = append(reasons, "batch/bulk operation pattern")
		}
		if signals["internal_service"] == "true" {
			reasons = append(reasons, "internal service-to-service communication")
		}
		if signals["schema_depth"] != "0" && signals["schema_depth"] != "1" {
			reasons = append(reasons, "complex nested payload schema")
		}
	}
	if len(reasons) == 0 {
		return fmt.Sprintf("%s preferred based on overall signal analysis.", protocol)
	}
	return fmt.Sprintf("%s preferred: %s.", protocol, strings.Join(reasons, "; "))
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// WriteReport writes the protocol recommendation as Markdown.
func WriteReport(report Report, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	md := renderMarkdown(report)
	return os.WriteFile(filepath.Join(outDir, "protocol_recommendation.md"), []byte(md), 0o644)
}

func renderMarkdown(report Report) string {
	var b strings.Builder
	b.WriteString("# Protocol Recommendation Report (SOAF Model)\n\n")
	b.WriteString(fmt.Sprintf("Total endpoints analyzed: **%d**\n", report.TotalEndpoints))
	b.WriteString(fmt.Sprintf("- REST preferred: %d\n", report.RESTPreferred))
	b.WriteString(fmt.Sprintf("- gRPC preferred: %d\n", report.GRPCPreferred))
	b.WriteString(fmt.Sprintf("- Either viable: %d\n", report.Either))
	b.WriteString(fmt.Sprintf("- Protobuf available: %v\n\n", report.HasProto))

	if len(report.Recommendations) == 0 {
		b.WriteString("No endpoints to analyze.\n")
		return b.String()
	}

	b.WriteString("## Per-Endpoint Recommendations\n\n")
	b.WriteString("| Endpoint | Protocol | Confidence | Rationale |\n")
	b.WriteString("| --- | --- | --- | --- |\n")
	for _, rec := range report.Recommendations {
		icon := "🔀"
		if rec.Protocol == "REST" {
			icon = "🌐"
		} else if rec.Protocol == "gRPC" {
			icon = "⚡"
		}
		b.WriteString(fmt.Sprintf("| `%s` | %s %s | %.0f%% | %s |\n",
			rec.Endpoint, icon, rec.Protocol, rec.Confidence*100, rec.Rationale))
	}

	b.WriteString("\n## Signal Legend\n\n")
	b.WriteString("| Signal | REST Favor | gRPC Favor |\n")
	b.WriteString("| --- | --- | --- |\n")
	b.WriteString("| CRUD pattern | +2.0 | |\n")
	b.WriteString("| Schema depth ≥3 | | +1.5 |\n")
	b.WriteString("| Streaming hint | | +3.0 |\n")
	b.WriteString("| Batch/bulk | | +2.0 |\n")
	b.WriteString("| Binary content | +2.0 | |\n")
	b.WriteString("| Idempotent (PUT/DELETE) | +0.5 | |\n")
	b.WriteString("| Public-facing | +1.5 | |\n")
	b.WriteString("| Internal service | | +1.5 |\n")
	b.WriteString("| No proto available | | ×0.3 penalty |\n")

	return b.String()
}

// Helper functions
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

func getString(data map[string]interface{}, key string) string {
	if v, ok := data[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getStringSlice(data map[string]interface{}, key string) []string {
	v, ok := data[key]
	if !ok {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	var result []string
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}
