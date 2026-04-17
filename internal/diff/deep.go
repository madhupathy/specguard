package diff

import (
	"encoding/json"
	"fmt"
	"os"
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

	baseServices := extractProtoServices(baseFiles)
	headServices := extractProtoServices(headFiles)

	for name := range baseServices {
		if _, ok := headServices[name]; !ok {
			changes = append(changes, Change{
				ID:       fmt.Sprintf("proto.service.removed::%s", name),
				Kind:     "service.removed",
				Severity: "breaking",
				Source:   name,
				Details:  map[string]string{"type": "protobuf", "service": name},
			})
		}
	}

	for name := range headServices {
		if _, ok := baseServices[name]; !ok {
			changes = append(changes, Change{
				ID:       fmt.Sprintf("proto.service.added::%s", name),
				Kind:     "service.added",
				Severity: "non_breaking",
				Source:   name,
				Details:  map[string]string{"type": "protobuf", "service": name},
			})
		}
	}

	baseMethods := extractProtoMethods(baseFiles)
	headMethods := extractProtoMethods(headFiles)

	for key := range baseMethods {
		if _, ok := headMethods[key]; !ok {
			changes = append(changes, Change{
				ID:       fmt.Sprintf("proto.method.removed::%s", key),
				Kind:     "method.removed",
				Severity: "breaking",
				Source:   key,
				Details:  map[string]string{"type": "protobuf", "method": key},
			})
		}
	}

	for key := range headMethods {
		if _, ok := baseMethods[key]; !ok {
			changes = append(changes, Change{
				ID:       fmt.Sprintf("proto.method.added::%s", key),
				Kind:     "method.added",
				Severity: "non_breaking",
				Source:   key,
				Details:  map[string]string{"type": "protobuf", "method": key},
			})
		}
	}

	return changes
}

// --- OpenAPI deep diff helpers ---

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
				ID:       fmt.Sprintf("method.removed::%s", fullPath),
				Kind:     "method.removed",
				Severity: "breaking",
				Source:   fullPath,
				Details:  map[string]string{"type": "openapi", "method": method, "path": path},
			})
			continue
		}

		if !baseHas && headHas {
			changes = append(changes, Change{
				ID:       fmt.Sprintf("method.added::%s", fullPath),
				Kind:     "method.added",
				Severity: "non_breaking",
				Source:   fullPath,
				Details:  map[string]string{"type": "openapi", "method": method, "path": path},
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

	baseParams := extractParamNames(base)
	headParams := extractParamNames(head)

	for name := range baseParams {
		if _, ok := headParams[name]; !ok {
			changes = append(changes, Change{
				ID:       fmt.Sprintf("parameter.removed::%s.%s", fullPath, name),
				Kind:     "parameter.removed",
				Severity: "breaking",
				Source:   fullPath,
				Details:  map[string]string{"type": "openapi", "parameter": name, "method": method, "path": path},
			})
		}
	}

	for name := range headParams {
		if _, ok := baseParams[name]; !ok {
			changes = append(changes, Change{
				ID:       fmt.Sprintf("parameter.added::%s.%s", fullPath, name),
				Kind:     "parameter.added",
				Severity: "non_breaking",
				Source:   fullPath,
				Details:  map[string]string{"type": "openapi", "parameter": name, "method": method, "path": path},
			})
		}
	}

	baseResponses := getMap(base, "responses")
	headResponses := getMap(head, "responses")

	for code := range baseResponses {
		if _, ok := headResponses[code]; !ok {
			changes = append(changes, Change{
				ID:       fmt.Sprintf("response.removed::%s.%s", fullPath, code),
				Kind:     "response.removed",
				Severity: "breaking",
				Source:   fullPath,
				Details:  map[string]string{"type": "openapi", "status_code": code, "method": method, "path": path},
			})
		}
	}

	for code := range headResponses {
		if _, ok := baseResponses[code]; !ok {
			changes = append(changes, Change{
				ID:       fmt.Sprintf("response.added::%s.%s", fullPath, code),
				Kind:     "response.added",
				Severity: "non_breaking",
				Source:   fullPath,
				Details:  map[string]string{"type": "openapi", "status_code": code, "method": method, "path": path},
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
				ID:       fmt.Sprintf("schema.removed::%s", name),
				Kind:     "schema.removed",
				Severity: "breaking",
				Source:   fmt.Sprintf("components.schemas.%s", name),
				Details:  map[string]string{"type": "openapi", "schema": name},
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
				ID:       fmt.Sprintf("schema.added::%s", name),
				Kind:     "schema.added",
				Severity: "non_breaking",
				Source:   fmt.Sprintf("components.schemas.%s", name),
				Details:  map[string]string{"type": "openapi", "schema": name},
			})
		}
	}

	return changes
}

func diffSchema(name string, base, head map[string]interface{}) []Change {
	var changes []Change

	baseRequired := getStringSlice(base, "required")
	headRequired := getStringSlice(head, "required")

	baseReqSet := toSet(baseRequired)
	headReqSet := toSet(headRequired)

	for field := range headReqSet {
		if _, ok := baseReqSet[field]; !ok {
			changes = append(changes, Change{
				ID:       fmt.Sprintf("schema.field.required::%s.%s", name, field),
				Kind:     "field.became_required",
				Severity: "breaking",
				Source:   fmt.Sprintf("components.schemas.%s.required.%s", name, field),
				Details:  map[string]string{"type": "openapi", "schema": name, "field": field},
			})
		}
	}

	for field := range baseReqSet {
		if _, ok := headReqSet[field]; !ok {
			changes = append(changes, Change{
				ID:       fmt.Sprintf("schema.field.optional::%s.%s", name, field),
				Kind:     "field.became_optional",
				Severity: "non_breaking",
				Source:   fmt.Sprintf("components.schemas.%s.required.%s", name, field),
				Details:  map[string]string{"type": "openapi", "schema": name, "field": field},
			})
		}
	}

	baseProps := getMap(base, "properties")
	headProps := getMap(head, "properties")

	for prop := range baseProps {
		if _, ok := headProps[prop]; !ok {
			changes = append(changes, Change{
				ID:       fmt.Sprintf("schema.property.removed::%s.%s", name, prop),
				Kind:     "property.removed",
				Severity: "breaking",
				Source:   fmt.Sprintf("components.schemas.%s.properties.%s", name, prop),
				Details:  map[string]string{"type": "openapi", "schema": name, "property": prop},
			})
		}
	}

	for prop := range headProps {
		if _, ok := baseProps[prop]; !ok {
			changes = append(changes, Change{
				ID:       fmt.Sprintf("schema.property.added::%s.%s", name, prop),
				Kind:     "property.added",
				Severity: "non_breaking",
				Source:   fmt.Sprintf("components.schemas.%s.properties.%s", name, prop),
				Details:  map[string]string{"type": "openapi", "schema": name, "property": prop},
			})
		}
	}

	for prop := range baseProps {
		headProp, ok := headProps[prop]
		if !ok {
			continue
		}
		basePropMap := asMap(baseProps[prop])
		headPropMap := asMap(headProp)
		if basePropMap == nil || headPropMap == nil {
			continue
		}
		baseType, _ := basePropMap["type"].(string)
		headType, _ := headPropMap["type"].(string)
		if baseType != "" && headType != "" && baseType != headType {
			changes = append(changes, Change{
				ID:       fmt.Sprintf("schema.property.type_changed::%s.%s", name, prop),
				Kind:     "property.type_changed",
				Severity: "breaking",
				Source:   fmt.Sprintf("components.schemas.%s.properties.%s", name, prop),
				Details:  map[string]string{"type": "openapi", "schema": name, "property": prop, "from_type": baseType, "to_type": headType},
			})
		}
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
				ID:       fmt.Sprintf("security.removed::%s", name),
				Kind:     "security.removed",
				Severity: "breaking",
				Source:   fmt.Sprintf("components.securitySchemes.%s", name),
				Details:  map[string]string{"type": "openapi", "security_scheme": name},
			})
		}
	}

	for name := range headSec {
		if _, ok := baseSec[name]; !ok {
			changes = append(changes, Change{
				ID:       fmt.Sprintf("security.added::%s", name),
				Kind:     "security.added",
				Severity: "potential_breaking",
				Source:   fmt.Sprintf("components.securitySchemes.%s", name),
				Details:  map[string]string{"type": "openapi", "security_scheme": name},
			})
		}
	}

	return changes
}

// --- Proto deep diff helpers ---

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

func extractParamNames(method map[string]interface{}) map[string]bool {
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

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}
