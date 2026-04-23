package export

import (
	"regexp"
	"strings"

	"github.com/Edthing/restlens-capture/internal/capture"
)

var (
	numericPattern = regexp.MustCompile(`^\d+$`)
	uuidPattern    = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	hexPattern     = regexp.MustCompile(`^[0-9a-fA-F]{16,}$`)
)

// GroupPatterns takes captured exchanges and groups them by inferred path patterns.
// Returns a map from (method, pattern) to the list of original paths.
//
// Each path is normalized per-segment: segments that look like IDs (numeric,
// UUID, or long hex) become {id}, every other segment is preserved verbatim.
// Paths that normalize to the same pattern are grouped; paths that don't are
// kept as separate operations. This is intentionally conservative — it's fine
// to under-group (two operations that could have been one) but never fine to
// over-group (merging genuinely distinct routes into a single operation),
// because over-grouping causes silent data loss in the exported spec.
func GroupPatterns(exchanges []capture.CapturedExchange) map[string][]string {
	// Collect unique (method, path) pairs, preserving first-seen order.
	type methodPath struct {
		method string
		path   string
	}
	seen := make(map[methodPath]bool)
	var paths []methodPath
	for _, ex := range exchanges {
		mp := methodPath{ex.Method, ex.Path}
		if !seen[mp] {
			seen[mp] = true
			paths = append(paths, mp)
		}
	}

	result := make(map[string][]string)
	for _, mp := range paths {
		pattern := normalizePath(mp.path)
		key := mp.method + " " + pattern
		result[key] = append(result[key], mp.path)
	}
	return result
}

// NormalizeSinglePath normalizes a single path by replacing ID-like segments.
func NormalizeSinglePath(path string) string {
	return normalizePath(path)
}

func normalizePath(path string) string {
	segs := splitPath(path)
	for i, s := range segs {
		if isLikelyID(s) {
			segs[i] = "{id}"
		}
	}
	if len(segs) == 0 {
		return "/"
	}
	return "/" + strings.Join(segs, "/")
}

func splitPath(path string) []string {
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func isLikelyID(segment string) bool {
	// Numeric IDs can be short (e.g. "1", "42", "123")
	if numericPattern.MatchString(segment) {
		return true
	}
	// Non-numeric patterns need minimum length to avoid false positives
	if len(segment) <= 3 {
		return false
	}
	if uuidPattern.MatchString(segment) {
		return true
	}
	if hexPattern.MatchString(segment) {
		return true
	}
	return false
}
