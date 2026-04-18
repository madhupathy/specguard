package diff

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// DeepCompare loads the actual normalized JSON files referenced by snapshot entries
// and performs semantic diffing of OpenAPI paths, schemas, parameters, and responses.
// It augments the shallow snapshot-level diff with fine-grained changes.
func DeepCompare(base, head Snapshot, repoPath string) Result {
	result := Compare(base, head)

	baseEntries := indexEntries(base.Entries)
	headEntries := indexEntries(head.Entries)

	for key, baseEntry := range baseEntries {
		headEntry, ok := headEntries[key]
		if !ok {
			continue
		}
		if baseEntry.SHA256 == headEntry.SHA256 {
			continue
		}

		switch baseEntry.Type {
		case "openapi":
			deepChanges := diffOpenAPIFiles(baseEntry, headEntry, repoPath)
			for i := range deepChanges {
				accumulate(&result, deepChanges[i])
			}
		case "protobuf":
			deepChanges := diffProtobufFiles(baseEntry, headEntry, repoPath)
			for i := range deepChanges {
				accumulate(&result, deepChanges[i])
			}
		}
	}

	ClassifyResult(&result)

	return result
}

func loadJSONFile(entry SnapshotEntry, repoPath string) (map[string]interface{}, error) {
	path := entry.Output
	if !strings.HasPrefix(path, "/") {
		path = repoPath + "/" + path
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// diffOpenAPIFiles performs deep comparison of two normalized OpenAPI JSON files.
func diffOpenAPIFiles(baseEntry, headEntry SnapshotEntry, repoPath string) []Change {
	baseData, err := loadJSONFile(baseEntry, repoPath)
	if err != nil {
		return nil
	}
	headData, err := loadJSONFile(headEntry, repoPath)
	if err != nil {
		return nil
	}

	var changes []Change
	changes = append(changes, diffPaths(baseData, headData)...)
	changes = append(changes, diffSchemas(baseData, headData)...)
	changes = append(changes, diffSecuritySchemes(baseData, headData)...)
	return changes
}

// diffProtobufFiles performs deep comparison of two normalized protobuf descriptor JSON files.
func diffProtobufFiles(baseEntry, headEntry SnapshotEntry, repoPath string) []Change {
	baseData, err := loadJSONFile(baseEntry, repoPath)
	if err != nil {
		return nil
	}
	headData, err := loadJSONFile(headEntry, repoPath)
	if err != nil {
		return nil
	}

	var changes []Change

	baseFiles := getSlice(baseData, "file")
	headFiles := getSlice(headData, "file")

	changes = append(changes, diffProtoServices(baseFiles, headFiles)...)
	changes = append(changes, diffProtoMethods(baseFiles, headFiles)...)
	changes = append(changes, diffProtoMessages(baseFiles, headFiles)...)

	return changes
}

// --- OpenAPI deep diff ---

func diffPaths(base, head map[string]interface{}) []Change {
	var changes []Change

	basePaths := getMap(base, "paths")
	headPaths := getMap(head, "paths")

	for path := range basePaths {
		if _, ok := headPaths[path]; !ok {
			changes = append(changes, Change{
				ID:       fmt.Sprintf("endpoint.removed::%s", path),
				Kind:     "endpoint.removed",
				Severity: "breaking",
				Source:   path,
				Details:  map[string]string{"type": "openapi", "path": path},
			})
			continue
		}
		basePathInfo := asMap(basePaths[path])
		headPathInfo := asMap(headPaths[path])
		if basePathInfo == nil || headPathInfo == nil {
			continue
		}
		changes = append(changes, diffEndpoint(path, basePathInfo, headPathInfo)...)
	}

	for path := range headPaths {
		if _, ok := basePaths[path]; !ok {
			changes = append(changes, Change{
				ID:       fmt.Sprintf("endpoint.added::%s", path),
				Kind:     "endpoint.added",
				Severity: "non_breaking",
				Source:   path,
				Details:  map[string]string{"type": "openapi", "path": path},
			})
		}
	}

	return changes
}

func diffEndpoint(path string, base, head map[string]interface{}) []Change {
	var changes []Change
	httpMethods := []string{"get", "post", "put", "delete", "patch", "head", "options"}

	for _, method := range httpMethods {
		_, baseHas := base[method]
		_, headHas := head[method]

		fullPath := fmt.Sprintf("%s %s", strings.ToUpper(method), path)

		if baseHas && !headHas {
			changes = append(changes, Change{
				ID: fmt.Sprintf("method.removed::%s", fullPath), Kind: "method.removed",
				Severity: "breaking", Source: fullPath,
				Details: map[string]string{"type": "openapi", "method": method, "path": path},
			})
			continue
		}
		if !baseHas && headHas {
			changes = append(changes, Change{
				ID: fmt.Sprintf("method.added::%s", fullPath), Kind: "method.added",
				Severity: "non_breaking", Source: fullPath,
				Details: map[string]string{"type": "openapi", "method": method, "path": path},
			})
			continue
		}
		if !baseHas || !headHas {
			continue
		}

		baseMethod := asMap(base[method])
		headMethod := asMap(head[method])
		if baseMethod == nil || headMethod == nil {
			continue
		}

		changes = append(changes, diffMethodDetail(fullPath, method, path, baseMethod, headMethod)...)
	}

	return changes
}

func diffMethodDetail(fullPath, method, path string, base, head map[string]interface{}) []Change {
	var changes []Change

	// --- Parameters ---
	baseParamMap := extractParamDetails(base)
	headParamMap := extractParamDetails(head)

	for name, baseParam := range baseParamMap {
		headParam, ok := headParamMap[name]
		if !ok {
			changes = append(changes, Change{
				ID:       fmt.Sprintf("parameter.removed::%s.%s", fullPath, name),
				Kind:     "parameter.removed", Severity: "breaking", Source: fullPath,
				Details: map[string]string{"type": "openapi", "parameter": name, "method": method, "path": path},
			})
			continue
		}
		// Check parameter location change (e.g. path -> query) — always breaking
		if baseParam["in"] != headParam["in"] && baseParam["in"] != "" && headParam["in"] != "" {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("parameter.location_changed::%s.%s", fullPath, name),
				Kind: "parameter.location_changed", Severity: "breaking", Source: fullPath,
				Details: map[string]string{
					"type": "openapi", "parameter": name, "method": method, "path": path,
					"from_in": baseParam["in"], "to_in": headParam["in"],
				},
			})
		}
		// Check parameter type change
		if baseParam["type"] != headParam["type"] && baseParam["type"] != "" && headParam["type"] != "" {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("parameter.type_changed::%s.%s", fullPath, name),
				Kind: "parameter.type_changed", Severity: "breaking", Source: fullPath,
				Details: map[string]string{
					"type": "openapi", "parameter": name, "method": method, "path": path,
					"from_type": baseParam["type"], "to_type": headParam["type"],
				},
			})
		}
		// Check required change (optional -> required is breaking)
		if baseParam["required"] != "true" && headParam["required"] == "true" {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("parameter.became_required::%s.%s", fullPath, name),
				Kind: "parameter.became_required", Severity: "breaking", Source: fullPath,
				Details: map[string]string{"type": "openapi", "parameter": name, "method": method, "path": path},
			})
		}
	}

	for name := range headParamMap {
		if _, ok := baseParamMap[name]; !ok {
			changes = append(changes, Change{
				ID:       fmt.Sprintf("parameter.added::%s.%s", fullPath, name),
				Kind:     "parameter.added", Severity: "non_breaking", Source: fullPath,
				Details: map[string]string{"type": "openapi", "parameter": name, "method": method, "path": path},
			})
		}
	}

	// --- Request body schema ---
	changes = append(changes, diffRequestBody(fullPath, method, path, base, head)...)

	// --- Responses ---
	baseResponses := getMap(base, "responses")
	headResponses := getMap(head, "responses")

	for code := range baseResponses {
		if _, ok := headResponses[code]; !ok {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("response.removed::%s.%s", fullPath, code),
				Kind: "response.removed", Severity: "breaking", Source: fullPath,
				Details: map[string]string{"type": "openapi", "status_code": code, "method": method, "path": path},
			})
		}
	}
	for code := range headResponses {
		if _, ok := baseResponses[code]; !ok {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("response.added::%s.%s", fullPath, code),
				Kind: "response.added", Severity: "non_breaking", Source: fullPath,
				Details: map[string]string{"type": "openapi", "status_code": code, "method": method, "path": path},
			})
		}
	}

	// Diff response schemas for existing codes
	for code, baseRespVal := range baseResponses {
		headRespVal, ok := headResponses[code]
		if !ok {
			continue
		}
		baseResp := asMap(baseRespVal)
		headResp := asMap(headRespVal)
		if baseResp == nil || headResp == nil {
			continue
		}
		changes = append(changes, diffResponseSchema(fullPath, method, path, code, baseResp, headResp)...)
	}

	return changes
}

// diffRequestBody detects schema changes in requestBody.
func diffRequestBody(fullPath, method, path string, base, head map[string]interface{}) []Change {
	var changes []Change

	baseBody := asMap(base["requestBody"])
	headBody := asMap(head["requestBody"])

	if baseBody == nil && headBody == nil {
		return nil
	}
	if baseBody != nil && headBody == nil {
		return []Change{{
			ID:   fmt.Sprintf("requestbody.removed::%s", fullPath),
			Kind: "request_body.removed", Severity: "breaking", Source: fullPath,
			Details: map[string]string{"type": "openapi", "method": method, "path": path},
		}}
	}
	if baseBody == nil && headBody != nil {
		return []Change{{
			ID:   fmt.Sprintf("requestbody.added::%s", fullPath),
			Kind: "request_body.added", Severity: "potential_breaking", Source: fullPath,
			Details: map[string]string{"type": "openapi", "method": method, "path": path},
		}}
	}

	// Check required flag
	baseRequired, _ := baseBody["required"].(bool)
	headRequired, _ := headBody["required"].(bool)
	if !baseRequired && headRequired {
		changes = append(changes, Change{
			ID:   fmt.Sprintf("requestbody.became_required::%s", fullPath),
			Kind: "request_body.became_required", Severity: "breaking", Source: fullPath,
			Details: map[string]string{"type": "openapi", "method": method, "path": path},
		})
	}

	// Diff inline schema properties within requestBody content
	baseContent := getMap(baseBody, "content")
	headContent := getMap(headBody, "content")
	for mediaType, baseMediaVal := range baseContent {
		headMediaVal, ok := headContent[mediaType]
		if !ok {
			continue
		}
		baseMedia := asMap(baseMediaVal)
		headMedia := asMap(headMediaVal)
		if baseMedia == nil || headMedia == nil {
			continue
		}
		baseSchema := asMap(baseMedia["schema"])
		headSchema := asMap(headMedia["schema"])
		if baseSchema == nil || headSchema == nil {
			continue
		}
		schemaChanges := diffInlineSchema(
			fmt.Sprintf("requestBody(%s)", mediaType),
			fullPath, method, path,
			baseSchema, headSchema,
		)
		changes = append(changes, schemaChanges...)
	}

	return changes
}

// diffResponseSchema diffs the response schema for a specific status code.
func diffResponseSchema(fullPath, method, path, code string, baseResp, headResp map[string]interface{}) []Change {
	var changes []Change
	baseContent := getMap(baseResp, "content")
	headContent := getMap(headResp, "content")
	for mediaType, baseMediaVal := range baseContent {
		headMediaVal, ok := headContent[mediaType]
		if !ok {
			continue
		}
		baseMedia := asMap(baseMediaVal)
		headMedia := asMap(headMediaVal)
		if baseMedia == nil || headMedia == nil {
			continue
		}
		baseSchema := asMap(baseMedia["schema"])
		headSchema := asMap(headMedia["schema"])
		if baseSchema == nil || headSchema == nil {
			continue
		}
		schemaChanges := diffInlineSchema(
			fmt.Sprintf("response[%s](%s)", code, mediaType),
			fullPath, method, path,
			baseSchema, headSchema,
		)
		changes = append(changes, schemaChanges...)
	}
	return changes
}

// diffInlineSchema checks properties, required fields, and enum values in an inline schema.
func diffInlineSchema(context, fullPath, method, path string, base, head map[string]interface{}) []Change {
	var changes []Change

	baseProps := getMap(base, "properties")
	headProps := getMap(head, "properties")

	for prop := range baseProps {
		if _, ok := headProps[prop]; !ok {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("inline_schema.property.removed::%s.%s.%s", fullPath, context, prop),
				Kind: "property.removed", Severity: "breaking", Source: fullPath,
				Details: map[string]string{
					"type": "openapi", "context": context,
					"property": prop, "method": method, "path": path,
				},
			})
		}
	}

	for prop := range headProps {
		if _, ok := baseProps[prop]; !ok {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("inline_schema.property.added::%s.%s.%s", fullPath, context, prop),
				Kind: "property.added", Severity: "non_breaking", Source: fullPath,
				Details: map[string]string{
					"type": "openapi", "context": context,
					"property": prop, "method": method, "path": path,
				},
			})
		}
	}

	// Prop type and enum changes
	for prop, basePropVal := range baseProps {
		headPropVal, ok := headProps[prop]
		if !ok {
			continue
		}
		baseProp := asMap(basePropVal)
		headProp := asMap(headPropVal)
		if baseProp == nil || headProp == nil {
			continue
		}
		bType, _ := baseProp["type"].(string)
		hType, _ := headProp["type"].(string)
		if bType != "" && hType != "" && bType != hType {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("inline_schema.property.type_changed::%s.%s.%s", fullPath, context, prop),
				Kind: "property.type_changed", Severity: "breaking", Source: fullPath,
				Details: map[string]string{
					"type": "openapi", "context": context,
					"property": prop, "from_type": bType, "to_type": hType,
				},
			})
		}
		// Enum changes
		changes = append(changes, diffEnumValues(fullPath, context, prop, baseProp, headProp)...)
	}

	// Required field changes
	baseRequired := toSet(getStringSlice(base, "required"))
	headRequired := toSet(getStringSlice(head, "required"))
	for field := range headRequired {
		if _, ok := baseRequired[field]; !ok {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("inline_schema.field.required::%s.%s.%s", fullPath, context, field),
				Kind: "field.became_required", Severity: "breaking", Source: fullPath,
				Details: map[string]string{"type": "openapi", "context": context, "field": field},
			})
		}
	}
	for field := range baseRequired {
		if _, ok := headRequired[field]; !ok {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("inline_schema.field.optional::%s.%s.%s", fullPath, context, field),
				Kind: "field.became_optional", Severity: "non_breaking", Source: fullPath,
				Details: map[string]string{"type": "openapi", "context": context, "field": field},
			})
		}
	}

	return changes
}

// diffEnumValues detects enum value removals (breaking) and additions (non-breaking).
func diffEnumValues(fullPath, context, prop string, baseProp, headProp map[string]interface{}) []Change {
	var changes []Change

	baseEnumRaw, hasBaseEnum := baseProp["enum"]
	headEnumRaw, hasHeadEnum := headProp["enum"]

	if !hasBaseEnum && !hasHeadEnum {
		return nil
	}
	if hasBaseEnum && !hasHeadEnum {
		// Removing enum constraint entirely — could be loosening or breaking depending on context
		changes = append(changes, Change{
			ID:   fmt.Sprintf("enum.constraint_removed::%s.%s.%s", fullPath, context, prop),
			Kind: "enum.constraint_removed", Severity: "potential_breaking", Source: fullPath,
			Details: map[string]string{"type": "openapi", "property": prop, "context": context},
		})
		return changes
	}

	baseEnum := toStringSet(baseEnumRaw)
	headEnum := toStringSet(headEnumRaw)

	// Removed enum values = BREAKING (clients may be sending/expecting removed values)
	for v := range baseEnum {
		if _, ok := headEnum[v]; !ok {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("enum.value_removed::%s.%s.%s.%s", fullPath, context, prop, v),
				Kind: "enum.value_removed", Severity: "breaking", Source: fullPath,
				Details: map[string]string{
					"type": "openapi", "property": prop, "context": context,
					"removed_value": v,
				},
			})
		}
	}

	// Added enum values = non-breaking (new valid value)
	for v := range headEnum {
		if _, ok := baseEnum[v]; !ok {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("enum.value_added::%s.%s.%s.%s", fullPath, context, prop, v),
				Kind: "enum.value_added", Severity: "non_breaking", Source: fullPath,
				Details: map[string]string{
					"type": "openapi", "property": prop, "context": context,
					"added_value": v,
				},
			})
		}
	}

	return changes
}

func diffSchemas(base, head map[string]interface{}) []Change {
	var changes []Change

	baseSchemas := getNestedMap(base, "components", "schemas")
	headSchemas := getNestedMap(head, "components", "schemas")

	for name := range baseSchemas {
		if _, ok := headSchemas[name]; !ok {
			changes = append(changes, Change{
				ID: fmt.Sprintf("schema.removed::%s", name), Kind: "schema.removed",
				Severity: "breaking", Source: fmt.Sprintf("components.schemas.%s", name),
				Details: map[string]string{"type": "openapi", "schema": name},
			})
			continue
		}
		baseSchema := asMap(baseSchemas[name])
		headSchema := asMap(headSchemas[name])
		if baseSchema == nil || headSchema == nil {
			continue
		}
		changes = append(changes, diffSchema(name, baseSchema, headSchema)...)
	}

	for name := range headSchemas {
		if _, ok := baseSchemas[name]; !ok {
			changes = append(changes, Change{
				ID: fmt.Sprintf("schema.added::%s", name), Kind: "schema.added",
				Severity: "non_breaking", Source: fmt.Sprintf("components.schemas.%s", name),
				Details: map[string]string{"type": "openapi", "schema": name},
			})
		}
	}

	return changes
}

func diffSchema(name string, base, head map[string]interface{}) []Change {
	var changes []Change

	baseRequired := toSet(getStringSlice(base, "required"))
	headRequired := toSet(getStringSlice(head, "required"))

	for field := range headRequired {
		if _, ok := baseRequired[field]; !ok {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("schema.field.required::%s.%s", name, field),
				Kind: "field.became_required", Severity: "breaking",
				Source:  fmt.Sprintf("components.schemas.%s.required.%s", name, field),
				Details: map[string]string{"type": "openapi", "schema": name, "field": field},
			})
		}
	}
	for field := range baseRequired {
		if _, ok := headRequired[field]; !ok {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("schema.field.optional::%s.%s", name, field),
				Kind: "field.became_optional", Severity: "non_breaking",
				Source:  fmt.Sprintf("components.schemas.%s.required.%s", name, field),
				Details: map[string]string{"type": "openapi", "schema": name, "field": field},
			})
		}
	}

	baseProps := getMap(base, "properties")
	headProps := getMap(head, "properties")

	for prop := range baseProps {
		if _, ok := headProps[prop]; !ok {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("schema.property.removed::%s.%s", name, prop),
				Kind: "property.removed", Severity: "breaking",
				Source:  fmt.Sprintf("components.schemas.%s.properties.%s", name, prop),
				Details: map[string]string{"type": "openapi", "schema": name, "property": prop},
			})
		}
	}
	for prop := range headProps {
		if _, ok := baseProps[prop]; !ok {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("schema.property.added::%s.%s", name, prop),
				Kind: "property.added", Severity: "non_breaking",
				Source:  fmt.Sprintf("components.schemas.%s.properties.%s", name, prop),
				Details: map[string]string{"type": "openapi", "schema": name, "property": prop},
			})
		}
	}

	for prop, basePropVal := range baseProps {
		headPropVal, ok := headProps[prop]
		if !ok {
			continue
		}
		baseProp := asMap(basePropVal)
		headProp := asMap(headPropVal)
		if baseProp == nil || headProp == nil {
			continue
		}
		bType, _ := baseProp["type"].(string)
		hType, _ := headProp["type"].(string)
		if bType != "" && hType != "" && bType != hType {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("schema.property.type_changed::%s.%s", name, prop),
				Kind: "property.type_changed", Severity: "breaking",
				Source:  fmt.Sprintf("components.schemas.%s.properties.%s", name, prop),
				Details: map[string]string{"type": "openapi", "schema": name, "property": prop, "from_type": bType, "to_type": hType},
			})
		}
		// Enum changes in component schemas
		changes = append(changes, diffEnumValues(
			fmt.Sprintf("components.schemas.%s", name), "component", prop, baseProp, headProp,
		)...)
	}

	return changes
}

func diffSecuritySchemes(base, head map[string]interface{}) []Change {
	var changes []Change

	baseSec := getNestedMap(base, "components", "securitySchemes")
	headSec := getNestedMap(head, "components", "securitySchemes")

	for name := range baseSec {
		if _, ok := headSec[name]; !ok {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("security.removed::%s", name),
				Kind: "security.removed", Severity: "breaking",
				Source:  fmt.Sprintf("components.securitySchemes.%s", name),
				Details: map[string]string{"type": "openapi", "security_scheme": name},
			})
		}
	}
	for name := range headSec {
		if _, ok := baseSec[name]; !ok {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("security.added::%s", name),
				Kind: "security.added", Severity: "potential_breaking",
				Source:  fmt.Sprintf("components.securitySchemes.%s", name),
				Details: map[string]string{"type": "openapi", "security_scheme": name},
			})
		}
	}

	return changes
}

// --- Proto deep diff ---

func diffProtoServices(baseFiles, headFiles []interface{}) []Change {
	var changes []Change
	baseServices := extractProtoServices(baseFiles)
	headServices := extractProtoServices(headFiles)

	for name := range baseServices {
		if _, ok := headServices[name]; !ok {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("proto.service.removed::%s", name),
				Kind: "service.removed", Severity: "breaking", Source: name,
				Details: map[string]string{"type": "protobuf", "service": name},
			})
		}
	}
	for name := range headServices {
		if _, ok := baseServices[name]; !ok {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("proto.service.added::%s", name),
				Kind: "service.added", Severity: "non_breaking", Source: name,
				Details: map[string]string{"type": "protobuf", "service": name},
			})
		}
	}
	return changes
}

func diffProtoMethods(baseFiles, headFiles []interface{}) []Change {
	var changes []Change
	baseMethods := extractProtoMethods(baseFiles)
	headMethods := extractProtoMethods(headFiles)

	for key := range baseMethods {
		if _, ok := headMethods[key]; !ok {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("proto.method.removed::%s", key),
				Kind: "method.removed", Severity: "breaking", Source: key,
				Details: map[string]string{"type": "protobuf", "method": key},
			})
		}
	}
	for key := range headMethods {
		if _, ok := baseMethods[key]; !ok {
			changes = append(changes, Change{
				ID:   fmt.Sprintf("proto.method.added::%s", key),
				Kind: "method.added", Severity: "non_breaking", Source: key,
				Details: map[string]string{"type": "protobuf", "method": key},
			})
		}
	}
	return changes
}

// diffProtoMessages checks for field number changes and field removals — the most
// critical proto breaking changes. Field number reuse or removal corrupts binary encoding.
func diffProtoMessages(baseFiles, headFiles []interface{}) []Change {
	var changes []Change

	baseMessages := extractProtoMessageFields(baseFiles)
	headMessages := extractProtoMessageFields(headFiles)

	for msgName, baseFields := range baseMessages {
		headFields, ok := headMessages[msgName]
		if !ok {
			// Whole message removed — already caught as schema change
			continue
		}

		// Index base fields by number
		baseByNum := map[string]protoField{}
		baseByName := map[string]protoField{}
		for _, f := range baseFields {
			baseByNum[f.Number] = f
			baseByName[f.Name] = f
		}

		headByNum := map[string]protoField{}
		headByName := map[string]protoField{}
		for _, f := range headFields {
			headByNum[f.Number] = f
			headByName[f.Name] = f
		}

		// Field removed by name
		for name, bf := range baseByName {
			if _, ok := headByName[name]; !ok {
				changes = append(changes, Change{
					ID:   fmt.Sprintf("proto.field.removed::%s.%s", msgName, name),
					Kind: "proto.field_removed", Severity: "breaking",
					Source: fmt.Sprintf("%s.%s", msgName, name),
					Details: map[string]string{
						"type": "protobuf", "message": msgName,
						"field": name, "field_number": bf.Number,
					},
				})
			}
		}

		// Field number reuse — most dangerous proto change
		for num, headField := range headByNum {
			if baseField, ok := baseByNum[num]; ok {
				if baseField.Name != headField.Name {
					changes = append(changes, Change{
						ID:   fmt.Sprintf("proto.field_number.reused::%s.%s", msgName, num),
						Kind: "proto.field_number_reused", Severity: "breaking",
						Source: fmt.Sprintf("%s field_number=%s", msgName, num),
						Details: map[string]string{
							"type": "protobuf", "message": msgName, "field_number": num,
							"old_name": baseField.Name, "new_name": headField.Name,
						},
					})
				}
				// Field type change
				if baseField.Type != headField.Type && baseField.Type != "" && headField.Type != "" {
					changes = append(changes, Change{
						ID:   fmt.Sprintf("proto.field.type_changed::%s.%s", msgName, headField.Name),
						Kind: "proto.field_type_changed", Severity: "breaking",
						Source: fmt.Sprintf("%s.%s", msgName, headField.Name),
						Details: map[string]string{
							"type": "protobuf", "message": msgName, "field": headField.Name,
							"from_type": baseField.Type, "to_type": headField.Type,
						},
					})
				}
			}
		}
	}

	return changes
}

type protoField struct {
	Name   string
	Number string
	Type   string
}

func extractProtoMessageFields(files []interface{}) map[string][]protoField {
	messages := map[string][]protoField{}
	for _, f := range files {
		fm := asMap(f)
		if fm == nil {
			continue
		}
		pkg, _ := fm["package"].(string)
		for _, mt := range getSlice(fm, "messageType") {
			mm := asMap(mt)
			if mm == nil {
				continue
			}
			name, _ := mm["name"].(string)
			if name == "" {
				continue
			}
			fullName := name
			if pkg != "" {
				fullName = pkg + "." + name
			}
			var fields []protoField
			for _, fld := range getSlice(mm, "field") {
				fm2 := asMap(fld)
				if fm2 == nil {
					continue
				}
				fName, _ := fm2["name"].(string)
				fNum := fmt.Sprintf("%v", fm2["number"])
				fType := fmt.Sprintf("%v", fm2["type"])
				fields = append(fields, protoField{Name: fName, Number: fNum, Type: fType})
			}
			messages[fullName] = fields
		}
	}
	return messages
}

func extractProtoServices(files []interface{}) map[string]bool {
	services := map[string]bool{}
	for _, f := range files {
		fm := asMap(f)
		if fm == nil {
			continue
		}
		pkg, _ := fm["package"].(string)
		for _, svc := range getSlice(fm, "service") {
			sm := asMap(svc)
			if sm == nil {
				continue
			}
			name, _ := sm["name"].(string)
			if name != "" {
				fullName := name
				if pkg != "" {
					fullName = pkg + "." + name
				}
				services[fullName] = true
			}
		}
	}
	return services
}

func extractProtoMethods(files []interface{}) map[string]bool {
	methods := map[string]bool{}
	for _, f := range files {
		fm := asMap(f)
		if fm == nil {
			continue
		}
		pkg, _ := fm["package"].(string)
		for _, svc := range getSlice(fm, "service") {
			sm := asMap(svc)
			if sm == nil {
				continue
			}
			svcName, _ := sm["name"].(string)
			for _, m := range getSlice(sm, "method") {
				mm := asMap(m)
				if mm == nil {
					continue
				}
				mName, _ := mm["name"].(string)
				if mName != "" {
					key := mName
					if svcName != "" {
						key = svcName + "." + mName
					}
					if pkg != "" {
						key = pkg + "." + key
					}
					methods[key] = true
				}
			}
		}
	}
	return methods
}

// --- Generic JSON helpers ---

func asMap(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return nil
}

func getMap(data map[string]interface{}, key string) map[string]interface{} {
	if v, ok := data[key]; ok {
		return asMap(v)
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

// extractParamDetails returns a map of param name → {in, type, required}
func extractParamDetails(method map[string]interface{}) map[string]map[string]string {
	params := map[string]map[string]string{}
	for _, p := range getSlice(method, "parameters") {
		pm := asMap(p)
		if pm == nil {
			continue
		}
		name, ok := pm["name"].(string)
		if !ok || name == "" {
			continue
		}
		in, _ := pm["in"].(string)
		required := "false"
		if r, ok := pm["required"].(bool); ok && r {
			required = "true"
		}
		typStr := ""
		if schema := asMap(pm["schema"]); schema != nil {
			typStr, _ = schema["type"].(string)
		}
		params[name] = map[string]string{"in": in, "type": typStr, "required": required}
	}
	return params
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}

func toStringSet(v interface{}) map[string]bool {
	s := map[string]bool{}
	arr, ok := v.([]interface{})
	if !ok {
		return s
	}
	for _, item := range arr {
		if str, ok := item.(string); ok {
			s[str] = true
		} else {
			s[fmt.Sprintf("%v", item)] = true
		}
	}
	return s
}

// sortedKeys returns map keys in sorted order (for deterministic output).
func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
