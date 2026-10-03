package llm

import (
	"encoding/json"
	"reflect"
	"strings"
)

// SchemaOf derives a JSON Schema from the Go type of v: a struct is an
// object whose properties are its json-tagged fields (a field without
// omitempty is required, unknown properties are refused), a slice is an
// array, a map is an object, and the scalar kinds map to their JSON type.
// A field's `desc` tag becomes its description. It is the one source of the
// schemas sent to a provider, so the contract is the Go struct the answer
// is decoded into, never a hand-written copy.
func SchemaOf(v any) json.RawMessage {
	raw, err := json.Marshal(schemaOfType(reflect.TypeOf(v), map[reflect.Type]bool{}))
	if err != nil {
		// Only maps of strings and nested maps are marshalled here.
		return json.RawMessage(`{}`)
	}
	return raw
}

// schemaOfType builds the schema node of t. seen breaks recursive types.
func schemaOfType(t reflect.Type, seen map[reflect.Type]bool) map[string]any {
	if t == nil {
		return map[string]any{}
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice, reflect.Array:
		return map[string]any{"type": "array", "items": schemaOfType(t.Elem(), seen)}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": schemaOfType(t.Elem(), seen)}
	case reflect.Struct:
		return structSchema(t, seen)
	}
	return map[string]any{}
}

// structSchema is the object schema of a struct type.
func structSchema(t reflect.Type, seen map[reflect.Type]bool) map[string]any {
	if seen[t] {
		return map[string]any{"type": "object"}
	}
	seen[t] = true
	defer delete(seen, t)
	properties := map[string]any{}
	required := []string{}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name, omitEmpty, skip := jsonFieldName(field)
		if skip {
			continue
		}
		node := schemaOfType(field.Type, seen)
		if desc := field.Tag.Get("desc"); desc != "" {
			node["description"] = desc
		}
		properties[name] = node
		if !omitEmpty {
			required = append(required, name)
		}
	}
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

// jsonFieldName reads a field's json tag the way encoding/json does.
func jsonFieldName(field reflect.StructField) (name string, omitEmpty, skip bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}
	parts := strings.Split(tag, ",")
	name = parts[0]
	if name == "" {
		name = field.Name
	}
	for _, opt := range parts[1:] {
		if opt == "omitempty" {
			omitEmpty = true
		}
	}
	return name, omitEmpty, false
}
