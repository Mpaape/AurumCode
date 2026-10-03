package deliberation

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// objectSchema is the part of a tool's JSON Schema the arguments are
// checked against: the declared properties and their JSON types, the
// required ones, and whether other properties are refused.
type objectSchema struct {
	Properties           map[string]propertySchema `json:"properties"`
	Required             []string                  `json:"required"`
	AdditionalProperties *bool                     `json:"additionalProperties"`
}

type propertySchema struct {
	Type string `json:"type"`
}

// ValidateArguments refuses arguments that are not a JSON object, that
// miss a required property, that carry a property the schema does not
// declare (when it closes the object) or whose value has another JSON
// type. It runs before a tool is ever executed.
func ValidateArguments(schema json.RawMessage, args json.RawMessage) error {
	if err := llm.ValidateToolArguments(args); err != nil {
		return err
	}
	var s objectSchema
	if len(schema) > 0 {
		if err := json.Unmarshal(schema, &s); err != nil {
			return fmt.Errorf("tool schema unreadable: %w", err)
		}
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(args, &obj); err != nil {
		return err
	}
	for _, name := range s.Required {
		if _, ok := obj[name]; !ok {
			return fmt.Errorf("missing required argument %q", name)
		}
	}
	names := make([]string, 0, len(obj))
	for name := range obj {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		prop, declared := s.Properties[name]
		if !declared {
			if s.AdditionalProperties != nil && !*s.AdditionalProperties {
				return fmt.Errorf("unknown argument %q", name)
			}
			continue
		}
		if !matchesType(prop.Type, obj[name]) {
			return fmt.Errorf("argument %q must be of type %s", name, prop.Type)
		}
	}
	return nil
}

// matchesType reports whether raw is a JSON value of the schema type want.
// An empty type accepts anything.
func matchesType(want string, raw json.RawMessage) bool {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}
	switch want {
	case "":
		return true
	case "string":
		_, ok := v.(string)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "number":
		_, ok := v.(float64)
		return ok
	case "integer":
		f, ok := v.(float64)
		return ok && f == float64(int64(f))
	case "array":
		_, ok := v.([]any)
		return ok
	case "object":
		_, ok := v.(map[string]any)
		return ok
	}
	return false
}
