package report

import (
	"sort"
	"time"

	"github.com/Edthing/restlens-capture/internal/capture"
)

type TrafficSummary struct {
	Endpoints []EndpointSummary `json:"endpoints"`
	TotalHits int               `json:"total_hits"`
	DateRange [2]time.Time      `json:"date_range"`
}

type EndpointSummary struct {
	Method      string      `json:"method"`
	Pattern     string      `json:"pattern"`
	HitCount    int         `json:"hit_count"`
	StatusCodes map[int]int `json:"status_codes"`
	AvgLatency  float64     `json:"avg_latency_ms"`
	P95Latency  float64     `json:"p95_latency_ms"`
}

// BuildSummary creates a traffic summary from exchanges and grouped patterns.
// patterns maps "METHOD /pattern" -> list of original paths that match.
func BuildSummary(exchanges []capture.CapturedExchange, patterns map[string][]string) TrafficSummary {
	if len(exchanges) == 0 {
		return TrafficSummary{}
	}

	// Build reverse lookup: (method, path) -> pattern key
	pathToPattern := make(map[string]string)
	for key, paths := range patterns {
		for _, p := range paths {
			// Extract method from key
			method := key[:len(key)-len(key)+len(key[:indexOf(key, ' ')])]
			pathToPattern[method+"|"+p] = key
		}
	}

	// Group exchanges by pattern
	type group struct {
		method      string
		pattern     string
		statusCodes map[int]int
		latencies   []int64
	}
	groups := make(map[string]*group)

	var minTime, maxTime time.Time
	for i, ex := range exchanges {
		if i == 0 {
			minTime = ex.Timestamp
			maxTime = ex.Timestamp
		} else {
			if ex.Timestamp.Before(minTime) {
				minTime = ex.Timestamp
			}
			if ex.Timestamp.After(maxTime) {
				maxTime = ex.Timestamp
			}
		}

		key := pathToPattern[ex.Method+"|"+ex.Path]
		if key == "" {
			key = ex.Method + " " + ex.Path
		}

		g, ok := groups[key]
		if !ok {
			idx := indexOf(key, ' ')
			g = &group{
				method:      key[:idx],
				pattern:     key[idx+1:],
				statusCodes: make(map[int]int),
			}
			groups[key] = g
		}

		g.statusCodes[ex.ResponseStatus]++
		g.latencies = append(g.latencies, ex.LatencyMs)
	}

	endpoints := make([]EndpointSummary, 0, len(groups))
	for _, g := range groups {
		sort.Slice(g.latencies, func(i, j int) bool { return g.latencies[i] < g.latencies[j] })

		var sum int64
		for _, l := range g.latencies {
			sum += l
		}

		endpoints = append(endpoints, EndpointSummary{
			Method:      g.method,
			Pattern:     g.pattern,
			HitCount:    len(g.latencies),
			StatusCodes: g.statusCodes,
			AvgLatency:  float64(sum) / float64(len(g.latencies)),
			P95Latency:  percentile(g.latencies, 0.95),
		})
	}

	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].HitCount > endpoints[j].HitCount
	})

	return TrafficSummary{
		Endpoints: endpoints,
		TotalHits: len(exchanges),
		DateRange: [2]time.Time{minTime, maxTime},
	}
}

func percentile(sorted []int64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return float64(sorted[0])
	}
	idx := int(float64(len(sorted)-1) * p)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return float64(sorted[idx])
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return len(s)
}
