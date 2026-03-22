package capture

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestInferSchema_Null(t *testing.T) {
	schema := InferSchema(nil)
	assertType(t, schema, "null")
}

func TestInferSchema_Boolean(t *testing.T) {
	schema := InferSchema(true)
	assertType(t, schema, "boolean")

	schema = InferSchema(false)
	assertType(t, schema, "boolean")
}

func TestInferSchema_Integer(t *testing.T) {
	schema := InferSchema(json.Number("42"))
	assertType(t, schema, "integer")
}

func TestInferSchema_Float(t *testing.T) {
	schema := InferSchema(json.Number("3.14"))
	assertType(t, schema, "number")
}

func TestInferSchema_Float64Integer(t *testing.T) {
	schema := InferSchema(float64(42))
	assertType(t, schema, "integer")
}

func TestInferSchema_Float64Decimal(t *testing.T) {
	schema := InferSchema(float64(3.14))
	assertType(t, schema, "number")
}

func TestInferSchema_String(t *testing.T) {
	schema := InferSchema("hello world")
	assertType(t, schema, "string")
}

func TestInferSchema_EmptyObject(t *testing.T) {
	schema := InferSchema(map[string]any{})
	assertType(t, schema, "object")

	props := schema["properties"].(map[string]any)
	if len(props) != 0 {
		t.Errorf("expected empty properties, got %v", props)
	}

	req := schema["required"].([]string)
	if len(req) != 0 {
		t.Errorf("expected empty required, got %v", req)
	}
}

func TestInferSchema_SimpleObject(t *testing.T) {
	obj := map[string]any{
		"name": "test",
		"age":  json.Number("25"),
	}
	schema := InferSchema(obj)
	assertType(t, schema, "object")

	props := schema["properties"].(map[string]any)
	if len(props) != 2 {
		t.Errorf("expected 2 properties, got %d", len(props))
	}

	nameSchema := props["name"].(map[string]any)
	assertType(t, nameSchema, "string")

	ageSchema := props["age"].(map[string]any)
	assertType(t, ageSchema, "integer")
}

func TestInferSchema_NestedObject(t *testing.T) {
	obj := map[string]any{
		"user": map[string]any{
			"profile": map[string]any{
				"email": "test@example.com",
			},
		},
	}
	schema := InferSchema(obj)
	assertType(t, schema, "object")

	userSchema := schema["properties"].(map[string]any)["user"].(map[string]any)
	assertType(t, userSchema, "object")

	profileSchema := userSchema["properties"].(map[string]any)["profile"].(map[string]any)
	assertType(t, profileSchema, "object")

	emailSchema := profileSchema["properties"].(map[string]any)["email"].(map[string]any)
	assertType(t, emailSchema, "string")
}

func TestInferSchema_ArrayOfObjects(t *testing.T) {
	arr := []any{
		map[string]any{"id": json.Number("1"), "name": "Alice"},
		map[string]any{"id": json.Number("2"), "name": "Bob"},
	}
	schema := InferSchema(arr)
	assertType(t, schema, "array")

	items := schema["items"].(map[string]any)
	assertType(t, items, "object")

	if schema["observedLength"] != 2 {
		t.Errorf("expected observedLength 2, got %v", schema["observedLength"])
	}
}

func TestInferSchema_EmptyArray(t *testing.T) {
	schema := InferSchema([]any{})
	assertType(t, schema, "array")

	items := schema["items"].(map[string]any)
	if len(items) != 0 {
		t.Errorf("expected empty items schema, got %v", items)
	}

	if schema["observedLength"] != 0 {
		t.Errorf("expected observedLength 0, got %v", schema["observedLength"])
	}
}

func TestInferSchema_MixedTypeArray(t *testing.T) {
	// Infers from first element only
	arr := []any{"hello", json.Number("42"), true}
	schema := InferSchema(arr)
	assertType(t, schema, "array")

	items := schema["items"].(map[string]any)
	assertType(t, items, "string")
}

func TestInferSchema_DeeplyNested(t *testing.T) {
	// 5+ levels deep
	obj := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": map[string]any{
					"d": map[string]any{
						"e": map[string]any{
							"value": json.Number("42"),
						},
					},
				},
			},
		},
	}
	schema := InferSchema(obj)

	// Traverse to the deepest level
	current := schema
	for _, key := range []string{"a", "b", "c", "d", "e"} {
		props := current["properties"].(map[string]any)
		current = props[key].(map[string]any)
		assertType(t, current, "object")
	}

	valueSchema := current["properties"].(map[string]any)["value"].(map[string]any)
	assertType(t, valueSchema, "integer")
}

func TestInferSchema_LargeObject(t *testing.T) {
	obj := make(map[string]any, 100)
	for i := 0; i < 100; i++ {
		obj[fmt.Sprintf("key_%d", i)] = fmt.Sprintf("value_%d", i)
	}
	schema := InferSchema(obj)
	assertType(t, schema, "object")

	props := schema["properties"].(map[string]any)
	if len(props) != 100 {
		t.Errorf("expected 100 properties, got %d", len(props))
	}
}

func TestInferSchema_Privacy_NoValuesInOutput(t *testing.T) {
	testData := map[string]any{
		"secret_key":   "sk-12345-very-secret",
		"password":     "hunter2",
		"email":        "alice@example.com",
		"credit_card":  "4111-1111-1111-1111",
		"ssn":          "123-45-6789",
		"access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
		"amount":       json.Number("99999"),
		"items": []any{
			map[string]any{"name": "Confidential Document", "price": json.Number("42.50")},
		},
	}

	schema := InferSchema(testData)
	serialized, _ := json.Marshal(schema)
	output := string(serialized)

	sensitiveValues := []string{
		"sk-12345-very-secret",
		"hunter2",
		"alice@example.com",
		"4111-1111-1111-1111",
		"123-45-6789",
		"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
		"99999",
		"42.50",
		"Confidential Document",
	}

	for _, val := range sensitiveValues {
		if strings.Contains(output, val) {
			t.Errorf("schema output contains sensitive value %q", val)
		}
	}
}

func TestInferSchemaFromBytes_ValidJSON(t *testing.T) {
	data := []byte(`{"name": "test", "count": 42}`)
	result := InferSchemaFromBytes(data)
	if result == nil {
		t.Fatal("expected non-nil schema")
	}

	var schema map[string]any
	json.Unmarshal(result, &schema)
	assertType(t, schema, "object")
}

func TestInferSchemaFromBytes_InvalidJSON(t *testing.T) {
	data := []byte(`not json at all`)
	result := InferSchemaFromBytes(data)
	if result != nil {
		t.Errorf("expected nil for invalid JSON, got %s", string(result))
	}
}

func TestInferSchemaFromBytes_EmptyInput(t *testing.T) {
	result := InferSchemaFromBytes(nil)
	if result != nil {
		t.Errorf("expected nil for empty input")
	}

	result = InferSchemaFromBytes([]byte{})
	if result != nil {
		t.Errorf("expected nil for empty bytes")
	}
}

func TestInferSchemaFromBytes_UsesNumberType(t *testing.T) {
	// json.Number should be used, so integers stay as "integer" not "number"
	data := []byte(`{"id": 12345}`)
	result := InferSchemaFromBytes(data)

	var schema map[string]any
	json.Unmarshal(result, &schema)
	props := schema["properties"].(map[string]any)
	idSchema := props["id"].(map[string]any)
	assertType(t, idSchema, "integer")
}

func TestInferSchemaFromBytes_ArrayOfStrings(t *testing.T) {
	data := []byte(`["a", "b", "c"]`)
	result := InferSchemaFromBytes(data)

	var schema map[string]any
	json.Unmarshal(result, &schema)
	assertType(t, schema, "array")

	items := schema["items"].(map[string]any)
	assertType(t, items, "string")
}

func assertType(t *testing.T, schema map[string]any, expected string) {
	t.Helper()
	got, ok := schema["type"].(string)
	if !ok {
		t.Errorf("expected type field to be string, got %T", schema["type"])
		return
	}
	if got != expected {
		t.Errorf("expected type %q, got %q", expected, got)
	}
}
