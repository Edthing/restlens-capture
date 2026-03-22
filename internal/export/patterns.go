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

// endpointKey is method + path pattern.
type endpointKey struct {
	Method  string
	Pattern string
}

// GroupPatterns takes captured exchanges and groups them by inferred path patterns.
// Returns a map from (method, pattern) to the list of original paths.
func GroupPatterns(exchanges []capture.CapturedExchange) map[string][]string {
	// Collect unique (method, path) pairs
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

	// Group by (method, segment count)
	type groupKey struct {
		method   string
		segCount int
	}
	groups := make(map[groupKey][]string)
	for _, mp := range paths {
		segs := splitPath(mp.path)
		gk := groupKey{mp.method, len(segs)}
		groups[gk] = append(groups[gk], mp.path)
	}

	// For each group, detect parameterized segments
	result := make(map[string][]string)
	for gk, groupPaths := range groups {
		if len(groupPaths) == 1 {
			pattern := normalizePath(groupPaths[0])
			key := gk.method + " " + pattern
			result[key] = groupPaths
			continue
		}

		segCount := gk.segCount
		segments := make([][]string, segCount)
		for _, p := range groupPaths {
			segs := splitPath(p)
			for i, s := range segs {
				if i < segCount {
					segments[i] = append(segments[i], s)
				}
			}
		}

		// Build pattern: replace high-cardinality ID-like segments
		patternSegs := make([]string, segCount)
		for i, segValues := range segments {
			if isParameterizedSegment(segValues) {
				patternSegs[i] = "{id}"
			} else {
				patternSegs[i] = segValues[0]
			}
		}

		pattern := "/" + strings.Join(patternSegs, "/")
		key := gk.method + " " + pattern
		result[key] = groupPaths
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

func isParameterizedSegment(values []string) bool {
	if len(values) <= 1 {
		return false
	}

	// Check uniqueness
	unique := make(map[string]bool)
	for _, v := range values {
		unique[v] = true
	}

	// Low cardinality = not a parameter
	if len(unique) <= 2 {
		return false
	}

	// Check if most values look like IDs
	idCount := 0
	for v := range unique {
		if isLikelyID(v) {
			idCount++
		}
	}

	return float64(idCount)/float64(len(unique)) > 0.5
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
