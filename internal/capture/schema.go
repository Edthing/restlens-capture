package capture

import (
	"bytes"
	"encoding/json"
	"regexp"
	"sort"
)

var (
	hexKeyPattern  = regexp.MustCompile(`^[0-9a-fA-F]{8,}$`)
	uuidKeyPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	numericKey     = regexp.MustCompile(`^\d+$`)
)

// InferSchema produces a JSON Schema from a decoded JSON value.
// It captures structure (types, keys, array shapes) but never stores actual values.
func InferSchema(v any) map[string]any {
	if v == nil {
		return map[string]any{"type": "null"}
	}

	switch val := v.(type) {
	case bool:
		return map[string]any{"type": "boolean"}

	case json.Number:
		if _, err := val.Int64(); err == nil {
			return map[string]any{"type": "integer"}
		}
		return map[string]any{"type": "number"}

	case float64:
		if val == float64(int64(val)) {
			return map[string]any{"type": "integer"}
		}
		return map[string]any{"type": "number"}

	case string:
		return map[string]any{"type": "string"}

	case []any:
		result := map[string]any{"type": "array"}
		if len(val) > 0 {
			result["items"] = InferSchema(val[0])
		} else {
			result["items"] = map[string]any{}
		}
		result["observedLength"] = len(val)
		return result

	case map[string]any:
		// Detect dynamic map keys (hashes, UUIDs, numeric IDs).
		// If most keys look like dynamic identifiers, emit additionalProperties
		// instead of treating each key as a fixed property.
		if len(val) > 1 && isDynamicMap(val) {
			// Infer value schema from first value
			for _, child := range val {
				return map[string]any{
					"type":                 "object",
					"additionalProperties": InferSchema(child),
				}
			}
		}

		properties := make(map[string]any, len(val))
		required := make([]string, 0, len(val))
		for k, child := range val {
			properties[k] = InferSchema(child)
			required = append(required, k)
		}
		sort.Strings(required)
		return map[string]any{
			"type":       "object",
			"properties": properties,
			"required":   required,
		}

	default:
		return map[string]any{"type": "unknown"}
	}
}

// isDynamicMap returns true if the object keys look like dynamic identifiers
// (hex hashes, UUIDs, numeric IDs) rather than fixed property names.
func isDynamicMap(m map[string]any) bool {
	dynamicCount := 0
	for k := range m {
		if hexKeyPattern.MatchString(k) || uuidKeyPattern.MatchString(k) || numericKey.MatchString(k) {
			dynamicCount++
		}
	}
	return float64(dynamicCount)/float64(len(m)) > 0.5
}

// InferSchemaFromBytes parses JSON bytes and infers a schema.
// Returns nil if the input is not valid JSON.
func InferSchemaFromBytes(data []byte) json.RawMessage {
	if len(data) == 0 {
		return nil
	}

	var v any
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := d.Decode(&v); err != nil {
		return nil
	}

	schema := InferSchema(v)
	b, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	return b
}
