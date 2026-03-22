package capture

import (
	"encoding/json"
	"time"
)

type CapturedExchange struct {
	ID                  string            `json:"id"`
	Timestamp           time.Time         `json:"timestamp"`
	Method              string            `json:"method"`
	Path                string            `json:"path"`
	QueryString         string            `json:"query_string,omitempty"`
	RequestHeaders      map[string]string `json:"request_headers,omitempty"`
	RequestBodySchema   json.RawMessage   `json:"request_body_schema,omitempty"`
	RequestContentType  string            `json:"request_content_type,omitempty"`
	ResponseStatus      int               `json:"response_status"`
	ResponseHeaders     map[string]string `json:"response_headers,omitempty"`
	ResponseBodySchema  json.RawMessage   `json:"response_body_schema,omitempty"`
	ResponseContentType string            `json:"response_content_type,omitempty"`
	LatencyMs           int64             `json:"latency_ms"`
}
