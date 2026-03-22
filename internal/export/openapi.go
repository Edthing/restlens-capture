package export

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Edthing/restlens-capture/internal/capture"
	"gopkg.in/yaml.v3"
)

// OpenAPISpec is a simplified OpenAPI 3.1 structure.
type OpenAPISpec struct {
	OpenAPI string                       `yaml:"openapi" json:"openapi"`
	Info    OpenAPIInfo                   `yaml:"info" json:"info"`
	Paths   map[string]map[string]OpItem `yaml:"paths" json:"paths"`
}

type OpenAPIInfo struct {
	Title       string `yaml:"title" json:"title"`
	Version     string `yaml:"version" json:"version"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

type OpItem struct {
	Summary   string                  `yaml:"summary" json:"summary"`
	Responses map[string]ResponseItem `yaml:"responses" json:"responses"`
	RequestBody *RequestBodyItem      `yaml:"requestBody,omitempty" json:"requestBody,omitempty"`
}

type ResponseItem struct {
	Description string                    `yaml:"description" json:"description"`
	Content     map[string]MediaTypeItem  `yaml:"content,omitempty" json:"content,omitempty"`
}

type RequestBodyItem struct {
	Content map[string]MediaTypeItem `yaml:"content" json:"content"`
}

type MediaTypeItem struct {
	Schema any `yaml:"schema,omitempty" json:"schema,omitempty"`
}

// GenerateOpenAPI builds an OpenAPI 3.1 spec from captured exchanges.
func GenerateOpenAPI(exchanges []capture.CapturedExchange) *OpenAPISpec {
	patterns := GroupPatterns(exchanges)

	// Map exchanges to their patterns
	type patternData struct {
		method     string
		pattern    string
		exchanges  []capture.CapturedExchange
	}

	patternMap := make(map[string]*patternData)
	for key, origPaths := range patterns {
		parts := strings.SplitN(key, " ", 2)
		method, pattern := strings.ToLower(parts[0]), parts[1]

		pathSet := make(map[string]bool)
		for _, p := range origPaths {
			pathSet[p] = true
		}

		pd := &patternData{method: method, pattern: pattern}
		for _, ex := range exchanges {
			if strings.EqualFold(ex.Method, parts[0]) && pathSet[ex.Path] {
				pd.exchanges = append(pd.exchanges, ex)
			}
		}
		patternMap[key] = pd
	}

	paths := make(map[string]map[string]OpItem)

	for _, pd := range patternMap {
		if _, ok := paths[pd.pattern]; !ok {
			paths[pd.pattern] = make(map[string]OpItem)
		}

		// Group responses by status code
		statusSchemas := make(map[int]json.RawMessage)
		var reqSchema json.RawMessage
		var reqContentType string

		for _, ex := range pd.exchanges {
			if ex.ResponseBodySchema != nil {
				if _, exists := statusSchemas[ex.ResponseStatus]; !exists {
					statusSchemas[ex.ResponseStatus] = ex.ResponseBodySchema
				}
			}
			if ex.RequestBodySchema != nil && reqSchema == nil {
				reqSchema = ex.RequestBodySchema
				reqContentType = ex.RequestContentType
			}
		}

		responses := make(map[string]ResponseItem)
		statusCodes := make([]int, 0, len(statusSchemas))
		for code := range statusSchemas {
			statusCodes = append(statusCodes, code)
		}
		sort.Ints(statusCodes)

		for _, code := range statusCodes {
			schema := statusSchemas[code]
			ri := ResponseItem{
				Description: fmt.Sprintf("Status %d", code),
			}
			if schema != nil {
				var s any
				json.Unmarshal(schema, &s)
				ri.Content = map[string]MediaTypeItem{
					"application/json": {Schema: s},
				}
			}
			responses[fmt.Sprintf("%d", code)] = ri
		}

		// Add any status codes without schemas
		for _, ex := range pd.exchanges {
			key := fmt.Sprintf("%d", ex.ResponseStatus)
			if _, exists := responses[key]; !exists {
				responses[key] = ResponseItem{
					Description: fmt.Sprintf("Status %d", ex.ResponseStatus),
				}
			}
		}

		op := OpItem{
			Summary:   fmt.Sprintf("%s %s", strings.ToUpper(pd.method), pd.pattern),
			Responses: responses,
		}

		if reqSchema != nil {
			ct := reqContentType
			if ct == "" {
				ct = "application/json"
			}
			var s any
			json.Unmarshal(reqSchema, &s)
			op.RequestBody = &RequestBodyItem{
				Content: map[string]MediaTypeItem{
					ct: {Schema: s},
				},
			}
		}

		paths[pd.pattern][pd.method] = op
	}

	return &OpenAPISpec{
		OpenAPI: "3.1.0",
		Info: OpenAPIInfo{
			Title:       "Inferred API",
			Version:     "1.0.0",
			Description: "Auto-generated from captured API traffic by restlens-capture",
		},
		Paths: paths,
	}
}

// WriteOpenAPIYAML writes the spec as YAML to the given writer.
func WriteOpenAPIYAML(spec *OpenAPISpec, w io.Writer) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	return enc.Encode(spec)
}
