package export

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Edthing/restlens-capture/internal/capture"
	"gopkg.in/yaml.v3"
)

func TestGenerateOpenAPI_ValidStructure(t *testing.T) {
	exchanges := []capture.CapturedExchange{
		{
			Method:              "GET",
			Path:                "/users/123",
			ResponseStatus:      200,
			ResponseContentType: "application/json",
			ResponseBodySchema:  json.RawMessage(`{"type":"object","properties":{"id":{"type":"integer"},"name":{"type":"string"}}}`),
		},
		{
			Method:              "GET",
			Path:                "/users/456",
			ResponseStatus:      200,
			ResponseContentType: "application/json",
			ResponseBodySchema:  json.RawMessage(`{"type":"object","properties":{"id":{"type":"integer"},"name":{"type":"string"}}}`),
		},
		{
			Method:              "GET",
			Path:                "/users/789",
			ResponseStatus:      404,
			ResponseContentType: "application/json",
			ResponseBodySchema:  json.RawMessage(`{"type":"object","properties":{"error":{"type":"string"}}}`),
		},
		{
			Method:              "GET",
			Path:                "/users/101",
			ResponseStatus:      200,
			ResponseContentType: "application/json",
			ResponseBodySchema:  json.RawMessage(`{"type":"object","properties":{"id":{"type":"integer"},"name":{"type":"string"}}}`),
		},
	}

	spec := GenerateOpenAPI(exchanges)

	if spec.OpenAPI != "3.1.0" {
		t.Errorf("expected OpenAPI 3.1.0, got %s", spec.OpenAPI)
	}

	if spec.Info.Title != "Inferred API" {
		t.Errorf("expected title 'Inferred API', got %s", spec.Info.Title)
	}

	if spec.Info.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %s", spec.Info.Version)
	}
}

func TestGenerateOpenAPI_PathsFromPatterns(t *testing.T) {
	exchanges := []capture.CapturedExchange{
		{Method: "GET", Path: "/users/123", ResponseStatus: 200},
		{Method: "GET", Path: "/users/456", ResponseStatus: 200},
		{Method: "GET", Path: "/users/789", ResponseStatus: 200},
		{Method: "GET", Path: "/users/101", ResponseStatus: 200},
	}

	spec := GenerateOpenAPI(exchanges)

	if _, ok := spec.Paths["/users/{id}"]; !ok {
		paths := make([]string, 0)
		for p := range spec.Paths {
			paths = append(paths, p)
		}
		t.Errorf("expected path /users/{id}, got: %v", paths)
	}
}

func TestGenerateOpenAPI_StatusCodes(t *testing.T) {
	exchanges := []capture.CapturedExchange{
		{Method: "GET", Path: "/items/1234", ResponseStatus: 200},
		{Method: "GET", Path: "/items/5678", ResponseStatus: 200},
		{Method: "GET", Path: "/items/9012", ResponseStatus: 404},
		{Method: "GET", Path: "/items/3456", ResponseStatus: 200},
	}

	spec := GenerateOpenAPI(exchanges)

	path := spec.Paths["/items/{id}"]
	if path == nil {
		t.Fatal("expected path /items/{id}")
	}

	op := path["get"]
	if _, ok := op.Responses["200"]; !ok {
		t.Error("expected 200 response")
	}
	if _, ok := op.Responses["404"]; !ok {
		t.Error("expected 404 response")
	}
}

func TestGenerateOpenAPI_RequestBodyIncluded(t *testing.T) {
	exchanges := []capture.CapturedExchange{
		{
			Method:              "POST",
			Path:                "/users",
			RequestContentType:  "application/json",
			RequestBodySchema:   json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`),
			ResponseStatus:      201,
			ResponseContentType: "application/json",
			ResponseBodySchema:  json.RawMessage(`{"type":"object","properties":{"id":{"type":"integer"}}}`),
		},
	}

	spec := GenerateOpenAPI(exchanges)

	path := spec.Paths["/users"]
	if path == nil {
		t.Fatal("expected path /users")
	}

	op := path["post"]
	if op.RequestBody == nil {
		t.Fatal("expected request body")
	}

	content := op.RequestBody.Content["application/json"]
	if content.Schema == nil {
		t.Error("expected request body schema")
	}
}

func TestGenerateOpenAPI_MultipleMethodsSamePath(t *testing.T) {
	exchanges := []capture.CapturedExchange{
		{Method: "GET", Path: "/users", ResponseStatus: 200},
		{Method: "POST", Path: "/users", ResponseStatus: 201},
	}

	spec := GenerateOpenAPI(exchanges)

	path := spec.Paths["/users"]
	if path == nil {
		t.Fatal("expected path /users")
	}

	if _, ok := path["get"]; !ok {
		t.Error("expected GET operation")
	}
	if _, ok := path["post"]; !ok {
		t.Error("expected POST operation")
	}
}

func TestGenerateOpenAPI_ResponseSchemaIncluded(t *testing.T) {
	exchanges := []capture.CapturedExchange{
		{
			Method:              "GET",
			Path:                "/health",
			ResponseStatus:      200,
			ResponseContentType: "application/json",
			ResponseBodySchema:  json.RawMessage(`{"type":"object","properties":{"status":{"type":"string"}}}`),
		},
	}

	spec := GenerateOpenAPI(exchanges)

	op := spec.Paths["/health"]["get"]
	resp := op.Responses["200"]
	content := resp.Content["application/json"]
	if content.Schema == nil {
		t.Error("expected response schema")
	}
}

func TestWriteOpenAPIYAML_ValidOutput(t *testing.T) {
	exchanges := []capture.CapturedExchange{
		{Method: "GET", Path: "/test", ResponseStatus: 200},
	}

	spec := GenerateOpenAPI(exchanges)

	var buf bytes.Buffer
	if err := WriteOpenAPIYAML(spec, &buf); err != nil {
		t.Fatalf("WriteOpenAPIYAML: %v", err)
	}

	// Verify it's valid YAML
	var parsed map[string]any
	if err := yaml.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid YAML output: %v", err)
	}

	if parsed["openapi"] != "3.1.0" {
		t.Errorf("expected openapi 3.1.0 in YAML, got %v", parsed["openapi"])
	}
}
