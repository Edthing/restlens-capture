package report

import (
	"encoding/json"
	"io"
)

// RenderJSON outputs the traffic summary as formatted JSON.
func RenderJSON(summary TrafficSummary, w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(summary)
}
