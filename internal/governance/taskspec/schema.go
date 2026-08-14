package taskspec

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// ValidateSchema checks the repository's JSON Schema contract before a caller
// uses it. The x-aurumcode keywords carry the cross-field rules that standard
// JSON Schema cannot express for arbitrary path strings.
func ValidateSchema(data []byte) error {
	root, err := jsonObject(data, "schema")
	if err != nil {
		return err
	}
	if value, ok := root["type"]; !ok || !jsonStringIs(value, "object") {
		return schemaError("type")
	}
	if value, ok := root["additionalProperties"]; !ok || !jsonBoolIs(value, false) {
		return schemaError("additionalProperties")
	}
	required, err := jsonStringArray(root["required"], "required")
	if err != nil {
		return err
	}
	for _, field := range requiredFields {
		if !contains(required, field) {
			return schemaError("required")
		}
	}
	properties, err := jsonObject(root["properties"], "properties")
	if err != nil {
		return err
	}
	if !propertyRef(properties, "mutation", "#/$defs/mutation") {
		return schemaError("properties.mutation")
	}
	if !propertyType(properties, "outcome", "string") {
		return schemaError("properties.outcome")
	}
	for _, field := range []string{"paths", "read_paths", "forbidden_paths"} {
		value, ok := properties[field]
		if !ok || !arrayItemsRef(value, "#/$defs/path") {
			return schemaError("properties." + field)
		}
	}
	defs, err := jsonObject(root["$defs"], "$defs")
	if err != nil {
		return err
	}
	mutation, err := jsonObject(defs["mutation"], "$defs.mutation")
	if err != nil {
		return err
	}
	if value, ok := mutation["additionalProperties"]; !ok || !jsonBoolIs(value, false) {
		return schemaError("$defs.mutation.additionalProperties")
	}
	mutationRequired, err := jsonStringArray(mutation["required"], "$defs.mutation.required")
	if err != nil {
		return err
	}
	for _, field := range []string{"id", "boundary", "change", "expected"} {
		if !contains(mutationRequired, field) {
			return schemaError("$defs.mutation.required")
		}
	}
	pathDefinition, err := jsonObject(defs["path"], "$defs.path")
	if err != nil {
		return err
	}
	pathPattern, ok := jsonString(pathDefinition["pattern"])
	if !ok || !strings.Contains(pathPattern, "(?!.*//)") ||
		!strings.Contains(pathPattern, "(?!.*\\s)") ||
		!strings.Contains(pathPattern, "(?:^|/)\\.\\.?") {
		return schemaError("$defs.path.pattern")
	}

	if err := validateSchemaBoundaryConditions(root["allOf"]); err != nil {
		return err
	}
	if err := validateSchemaExtensions(root["x-aurumcode"]); err != nil {
		return err
	}
	return nil
}

func validateSchemaBoundaryConditions(raw json.RawMessage) error {
	var conditions []json.RawMessage
	if err := json.Unmarshal(raw, &conditions); err != nil {
		return schemaError("allOf")
	}
	for boundary := range boundaryValues {
		found := false
		for _, condition := range conditions {
			if schemaBoundaryCondition(condition, boundary) {
				found = true
				break
			}
		}
		if !found {
			return schemaError("allOf")
		}
	}
	return nil
}

func schemaBoundaryCondition(raw json.RawMessage, boundary string) bool {
	condition, err := jsonObject(raw, "allOf.condition")
	if err != nil {
		return false
	}
	ifObject, err := jsonObject(condition["if"], "allOf.if")
	if err != nil {
		return false
	}
	ifProperties, err := jsonObject(ifObject["properties"], "allOf.if.properties")
	if err != nil {
		return false
	}
	mutation, err := jsonObject(ifProperties["mutation"], "allOf.if.mutation")
	if err != nil {
		return false
	}
	mutationProperties, err := jsonObject(mutation["properties"], "allOf.if.mutation.properties")
	if err != nil {
		return false
	}
	boundaryObject, err := jsonObject(mutationProperties["boundary"], "allOf.if.boundary")
	if err != nil {
		return false
	}
	if value, ok := boundaryObject["const"]; !ok || !jsonStringIs(value, boundary) {
		return false
	}
	thenObject, err := jsonObject(condition["then"], "allOf.then")
	if err != nil {
		return false
	}
	thenProperties, err := jsonObject(thenObject["properties"], "allOf.then.properties")
	if err != nil {
		return false
	}
	trustBoundaries, err := jsonObject(thenProperties["trust_boundaries"], "allOf.then.trust_boundaries")
	if err != nil {
		return false
	}
	containsObject, err := jsonObject(trustBoundaries["contains"], "allOf.then.contains")
	if err != nil {
		return false
	}
	value, ok := containsObject["const"]
	return ok && jsonStringIs(value, boundary)
}

func validateSchemaExtensions(raw json.RawMessage) error {
	extension, err := jsonObject(raw, "x-aurumcode")
	if err != nil {
		return err
	}
	pathPolicy, err := jsonObject(extension["path_policy"], "x-aurumcode.path_policy")
	if err != nil {
		return err
	}
	for _, name := range []string{"relative_posix", "reject_whitespace", "reject_control", "reject_empty_components"} {
		if value, ok := pathPolicy[name]; !ok || !jsonBoolIs(value, true) {
			return schemaError("x-aurumcode.path_policy." + name)
		}
	}
	disjoint, err := jsonObject(extension["scope_disjointness"], "x-aurumcode.scope_disjointness")
	if err != nil {
		return err
	}
	fields, err := jsonStringArray(disjoint["fields"], "x-aurumcode.scope_disjointness.fields")
	if err != nil || len(fields) != 3 || fields[0] != "paths" || fields[1] != "read_paths" || fields[2] != "forbidden_paths" {
		return schemaError("x-aurumcode.scope_disjointness.fields")
	}
	relation, ok := jsonString(disjoint["relation"])
	if !ok || relation != "no-equal-or-ancestor" {
		return schemaError("x-aurumcode.scope_disjointness.relation")
	}
	return nil
}

func jsonObject(raw json.RawMessage, field string) (map[string]json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, schemaError(field)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var object map[string]json.RawMessage
	if err := decoder.Decode(&object); err != nil || object == nil {
		return nil, schemaError(field)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, schemaError(field)
	}
	return object, nil
}

func jsonString(raw json.RawMessage) (string, bool) {
	var value string
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	return value, true
}

func jsonStringIs(raw json.RawMessage, expected string) bool {
	value, ok := jsonString(raw)
	return ok && value == expected
}

func jsonBoolIs(raw json.RawMessage, expected bool) bool {
	var value bool
	return len(raw) > 0 && json.Unmarshal(raw, &value) == nil && value == expected
}

func jsonStringArray(raw json.RawMessage, field string) ([]string, error) {
	var values []string
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return nil, schemaError(field)
	}
	return values, nil
}

func propertyRef(properties map[string]json.RawMessage, name, ref string) bool {
	property, err := jsonObject(properties[name], "property")
	if err != nil {
		return false
	}
	return jsonStringIs(property["$ref"], ref)
}

func propertyType(properties map[string]json.RawMessage, name, wanted string) bool {
	property, err := jsonObject(properties[name], "property")
	if err != nil {
		return false
	}
	return jsonStringIs(property["type"], wanted)
}

func arrayItemsRef(raw json.RawMessage, ref string) bool {
	property, err := jsonObject(raw, "array")
	if err != nil || !jsonStringIs(property["type"], "array") {
		return false
	}
	items, err := jsonObject(property["items"], "array.items")
	return err == nil && jsonStringIs(items["$ref"], ref)
}

func schemaError(field string) *FieldError {
	return fieldError("schema."+field, CodeInvalidField, "schema contract is incomplete")
}
