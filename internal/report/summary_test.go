package report

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/Edthing/restlens-capture/internal/capture"
)

func TestBuildSummary_Empty(t *testing.T) {
	summary := BuildSummary(nil, nil)
	if summary.TotalHits != 0 {
		t.Errorf("expected 0 hits, got %d", summary.TotalHits)
	}
	if len(summary.Endpoints) != 0 {
		t.Errorf("expected 0 endpoints, got %d", len(summary.Endpoints))
	}
}

func TestBuildSummary_SingleEndpoint(t *testing.T) {
	now := time.Now().UTC()
	exchanges := []capture.CapturedExchange{
		{Method: "GET", Path: "/api/users", ResponseStatus: 200, LatencyMs: 10, Timestamp: now},
		{Method: "GET", Path: "/api/users", ResponseStatus: 200, LatencyMs: 20, Timestamp: now.Add(time.Second)},
		{Method: "GET", Path: "/api/users", ResponseStatus: 200, LatencyMs: 30, Timestamp: now.Add(2 * time.Second)},
	}

	patterns := map[string][]string{
		"GET /api/users": {"/api/users"},
	}

	summary := BuildSummary(exchanges, patterns)

	if summary.TotalHits != 3 {
		t.Errorf("expected 3 hits, got %d", summary.TotalHits)
	}

	if len(summary.Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(summary.Endpoints))
	}

	ep := summary.Endpoints[0]
	if ep.Method != "GET" {
		t.Errorf("expected GET, got %s", ep.Method)
	}
	if ep.Pattern != "/api/users" {
		t.Errorf("expected /api/users, got %s", ep.Pattern)
	}
	if ep.HitCount != 3 {
		t.Errorf("expected 3 hits, got %d", ep.HitCount)
	}
	if ep.StatusCodes[200] != 3 {
		t.Errorf("expected 3 status 200, got %d", ep.StatusCodes[200])
	}
}

func TestBuildSummary_MultipleEndpoints_SortedByHitCount(t *testing.T) {
	now := time.Now().UTC()
	exchanges := []capture.CapturedExchange{
		{Method: "GET", Path: "/a", ResponseStatus: 200, LatencyMs: 10, Timestamp: now},
		{Method: "POST", Path: "/b", ResponseStatus: 201, LatencyMs: 20, Timestamp: now},
		{Method: "POST", Path: "/b", ResponseStatus: 201, LatencyMs: 30, Timestamp: now},
		{Method: "POST", Path: "/b", ResponseStatus: 201, LatencyMs: 40, Timestamp: now},
		{Method: "DELETE", Path: "/c", ResponseStatus: 204, LatencyMs: 5, Timestamp: now},
		{Method: "DELETE", Path: "/c", ResponseStatus: 204, LatencyMs: 15, Timestamp: now},
	}

	patterns := map[string][]string{
		"GET /a":    {"/a"},
		"POST /b":   {"/b"},
		"DELETE /c": {"/c"},
	}

	summary := BuildSummary(exchanges, patterns)

	if len(summary.Endpoints) != 3 {
		t.Fatalf("expected 3 endpoints, got %d", len(summary.Endpoints))
	}

	// Should be sorted by hit count descending
	if summary.Endpoints[0].HitCount != 3 {
		t.Errorf("first endpoint should have 3 hits, got %d", summary.Endpoints[0].HitCount)
	}
	if summary.Endpoints[1].HitCount != 2 {
		t.Errorf("second endpoint should have 2 hits, got %d", summary.Endpoints[1].HitCount)
	}
	if summary.Endpoints[2].HitCount != 1 {
		t.Errorf("third endpoint should have 1 hit, got %d", summary.Endpoints[2].HitCount)
	}
}

func TestBuildSummary_LatencyStats(t *testing.T) {
	now := time.Now().UTC()
	// 20 requests with latencies 1..20
	var exchanges []capture.CapturedExchange
	for i := 1; i <= 20; i++ {
		exchanges = append(exchanges, capture.CapturedExchange{
			Method:         "GET",
			Path:           "/api/test",
			ResponseStatus: 200,
			LatencyMs:      int64(i),
			Timestamp:      now,
		})
	}

	patterns := map[string][]string{
		"GET /api/test": {"/api/test"},
	}

	summary := BuildSummary(exchanges, patterns)
	ep := summary.Endpoints[0]

	// Avg should be (1+2+...+20)/20 = 210/20 = 10.5
	if ep.AvgLatency != 10.5 {
		t.Errorf("expected avg 10.5, got %f", ep.AvgLatency)
	}

	// P95 of sorted [1..20] at index 19*0.95 = 18.05 -> index 18 -> value 19
	if ep.P95Latency != 19 {
		t.Errorf("expected p95 19, got %f", ep.P95Latency)
	}
}

func TestBuildSummary_StatusCodeDistribution(t *testing.T) {
	now := time.Now().UTC()
	exchanges := []capture.CapturedExchange{
		{Method: "GET", Path: "/api/x", ResponseStatus: 200, LatencyMs: 10, Timestamp: now},
		{Method: "GET", Path: "/api/x", ResponseStatus: 200, LatencyMs: 10, Timestamp: now},
		{Method: "GET", Path: "/api/x", ResponseStatus: 404, LatencyMs: 5, Timestamp: now},
		{Method: "GET", Path: "/api/x", ResponseStatus: 500, LatencyMs: 100, Timestamp: now},
	}

	patterns := map[string][]string{
		"GET /api/x": {"/api/x"},
	}

	summary := BuildSummary(exchanges, patterns)
	ep := summary.Endpoints[0]

	if ep.StatusCodes[200] != 2 {
		t.Errorf("expected 2x 200, got %d", ep.StatusCodes[200])
	}
	if ep.StatusCodes[404] != 1 {
		t.Errorf("expected 1x 404, got %d", ep.StatusCodes[404])
	}
	if ep.StatusCodes[500] != 1 {
		t.Errorf("expected 1x 500, got %d", ep.StatusCodes[500])
	}
}

func TestBuildSummary_DateRange(t *testing.T) {
	t1 := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 3, 21, 15, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 3, 22, 8, 0, 0, 0, time.UTC)

	exchanges := []capture.CapturedExchange{
		{Method: "GET", Path: "/a", ResponseStatus: 200, LatencyMs: 10, Timestamp: t2},
		{Method: "GET", Path: "/a", ResponseStatus: 200, LatencyMs: 10, Timestamp: t1},
		{Method: "GET", Path: "/a", ResponseStatus: 200, LatencyMs: 10, Timestamp: t3},
	}

	patterns := map[string][]string{
		"GET /a": {"/a"},
	}

	summary := BuildSummary(exchanges, patterns)

	if !summary.DateRange[0].Equal(t1) {
		t.Errorf("expected min time %v, got %v", t1, summary.DateRange[0])
	}
	if !summary.DateRange[1].Equal(t3) {
		t.Errorf("expected max time %v, got %v", t3, summary.DateRange[1])
	}
}

func TestRenderJSON_ValidOutput(t *testing.T) {
	summary := TrafficSummary{
		TotalHits: 5,
		Endpoints: []EndpointSummary{
			{
				Method:      "GET",
				Pattern:     "/api/test",
				HitCount:    5,
				StatusCodes: map[int]int{200: 4, 404: 1},
				AvgLatency:  25.0,
				P95Latency:  50.0,
			},
		},
		DateRange: [2]time.Time{
			time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
		},
	}

	var buf bytes.Buffer
	if err := RenderJSON(summary, &buf); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}

	// Verify it's valid JSON
	var parsed TrafficSummary
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	if parsed.TotalHits != 5 {
		t.Errorf("expected 5 hits, got %d", parsed.TotalHits)
	}
	if len(parsed.Endpoints) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(parsed.Endpoints))
	}
}

func TestRenderTerminal_NoTraffic(t *testing.T) {
	var buf bytes.Buffer
	RenderTerminal(TrafficSummary{}, &buf)

	output := buf.String()
	if !containsStr(output, "No traffic captured") {
		t.Errorf("expected 'No traffic captured' message, got: %s", output)
	}
}

func TestRenderTerminal_WithTraffic(t *testing.T) {
	summary := TrafficSummary{
		TotalHits: 10,
		Endpoints: []EndpointSummary{
			{
				Method:      "GET",
				Pattern:     "/api/users",
				HitCount:    10,
				StatusCodes: map[int]int{200: 8, 404: 2},
				AvgLatency:  25.0,
				P95Latency:  50.0,
			},
		},
		DateRange: [2]time.Time{
			time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
		},
	}

	var buf bytes.Buffer
	RenderTerminal(summary, &buf)

	output := buf.String()
	if !containsStr(output, "10 requests") {
		t.Errorf("expected '10 requests' in output, got: %s", output)
	}
	if !containsStr(output, "/api/users") {
		t.Errorf("expected '/api/users' in output, got: %s", output)
	}
}

func containsStr(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
