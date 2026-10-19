package attempt

import (
	"encoding/json"
	"fmt"
)

// validateArgsAgainstSchema checks bound tool arguments against the catalog JSON Schema.
// Supports the object / properties / required / type / additionalProperties subset used in fixtures.
func validateArgsAgainstSchema(schema, args json.RawMessage) error {
	if len(schema) == 0 {
		return nil
	}
	var sch map[string]any
	if err := json.Unmarshal(schema, &sch); err != nil {
		return fmt.Errorf("invalid parameters schema: %w", err)
	}
	var data any
	if len(args) == 0 {
		data = map[string]any{}
	} else if err := json.Unmarshal(args, &data); err != nil {
		return fmt.Errorf("arguments are not JSON: %w", err)
	}
	return validateNode(sch, data, "")
}

func validateNode(schema map[string]any, data any, path string) error {
	if t, ok := schema["type"].(string); ok {
		if err := checkType(t, data, path); err != nil {
			return err
		}
	}
	if t, _ := schema["type"].(string); t == "object" || schema["properties"] != nil {
		obj, ok := data.(map[string]any)
		if !ok {
			if data == nil {
				obj = map[string]any{}
			} else {
				return fmt.Errorf("%s: expected object", label(path))
			}
		}
		props, _ := schema["properties"].(map[string]any)
		if req, ok := schema["required"].([]any); ok {
			for _, item := range req {
				name, _ := item.(string)
				if name == "" {
					continue
				}
				if _, exists := obj[name]; !exists {
					return fmt.Errorf("%s: missing required property %q", label(path), name)
				}
			}
		}
		if additional, ok := schema["additionalProperties"].(bool); ok && !additional {
			for key := range obj {
				if props != nil {
					if _, exists := props[key]; !exists {
						return fmt.Errorf("%s: additional property %q not allowed", label(path), key)
					}
				}
			}
		}
		for key, propSchema := range props {
			ps, ok := propSchema.(map[string]any)
			if !ok {
				continue
			}
			v, exists := obj[key]
			if !exists {
				continue
			}
			child := key
			if path != "" {
				child = path + "." + key
			}
			if err := validateNode(ps, v, child); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkType(want string, data any, path string) error {
	switch want {
	case "object":
		if _, ok := data.(map[string]any); !ok && data != nil {
			return fmt.Errorf("%s: expected object", label(path))
		}
	case "string":
		if _, ok := data.(string); !ok {
			return fmt.Errorf("%s: expected string", label(path))
		}
	case "number":
		switch data.(type) {
		case float64, json.Number:
			// ok
		default:
			return fmt.Errorf("%s: expected number", label(path))
		}
	case "integer":
		switch v := data.(type) {
		case float64:
			if v != float64(int64(v)) {
				return fmt.Errorf("%s: expected integer", label(path))
			}
		case json.Number:
			// ok
		default:
			return fmt.Errorf("%s: expected integer", label(path))
		}
	case "boolean":
		if _, ok := data.(bool); !ok {
			return fmt.Errorf("%s: expected boolean", label(path))
		}
	case "array":
		if _, ok := data.([]any); !ok {
			return fmt.Errorf("%s: expected array", label(path))
		}
	case "null":
		if data != nil {
			return fmt.Errorf("%s: expected null", label(path))
		}
	}
	return nil
}

func label(path string) string {
	if path == "" {
		return "arguments"
	}
	return "arguments." + path
}
