package export

import (
	"testing"

	"github.com/Edthing/restlens-capture/internal/capture"
)

func TestGroupPatterns_NumericIDs(t *testing.T) {
	exchanges := makeExchanges(
		"GET", "/users/123",
		"GET", "/users/456",
		"GET", "/users/789",
		"GET", "/users/101",
	)

	patterns := GroupPatterns(exchanges)

	key := "GET /users/{id}"
	if _, ok := patterns[key]; !ok {
		t.Errorf("expected pattern %q, got keys: %v", key, patternKeys(patterns))
	}
}

func TestGroupPatterns_NestedNumericIDs(t *testing.T) {
	exchanges := makeExchanges(
		"GET", "/users/123/posts/789",
		"GET", "/users/456/posts/101",
		"GET", "/users/789/posts/202",
		"GET", "/users/101/posts/303",
	)

	patterns := GroupPatterns(exchanges)

	key := "GET /users/{id}/posts/{id}"
	if _, ok := patterns[key]; !ok {
		t.Errorf("expected pattern %q, got keys: %v", key, patternKeys(patterns))
	}
}

func TestGroupPatterns_UUIDs(t *testing.T) {
	exchanges := makeExchanges(
		"GET", "/items/550e8400-e29b-41d4-a716-446655440000",
		"GET", "/items/6ba7b810-9dad-11d1-80b4-00c04fd430c8",
		"GET", "/items/7c9e6679-7425-40de-944b-e07fc1f90ae7",
		"GET", "/items/f47ac10b-58cc-4372-a567-0e02b2c3d479",
	)

	patterns := GroupPatterns(exchanges)

	key := "GET /items/{id}"
	if _, ok := patterns[key]; !ok {
		t.Errorf("expected pattern %q, got keys: %v", key, patternKeys(patterns))
	}
}

func TestGroupPatterns_StaticPreserved(t *testing.T) {
	exchanges := makeExchanges(
		"GET", "/api/v1/health",
		"GET", "/api/v1/health",
	)

	patterns := GroupPatterns(exchanges)

	key := "GET /api/v1/health"
	if _, ok := patterns[key]; !ok {
		t.Errorf("expected pattern %q, got keys: %v", key, patternKeys(patterns))
	}
}

func TestGroupPatterns_MixedStaticAndDynamic(t *testing.T) {
	exchanges := makeExchanges(
		"GET", "/api/v1/users/123/profile",
		"GET", "/api/v1/users/456/profile",
		"GET", "/api/v1/users/789/profile",
		"GET", "/api/v1/users/101/profile",
	)

	patterns := GroupPatterns(exchanges)

	key := "GET /api/v1/users/{id}/profile"
	if _, ok := patterns[key]; !ok {
		t.Errorf("expected pattern %q, got keys: %v", key, patternKeys(patterns))
	}
}

func TestGroupPatterns_ShortAlphaNotParameterized(t *testing.T) {
	exchanges := makeExchanges(
		"GET", "/users/me",
	)

	patterns := GroupPatterns(exchanges)

	// "me" is too short to be parameterized
	key := "GET /users/me"
	if _, ok := patterns[key]; !ok {
		t.Errorf("expected pattern %q, got keys: %v", key, patternKeys(patterns))
	}
}

func TestGroupPatterns_ShortAlphaNotParameterized_Lang(t *testing.T) {
	exchanges := makeExchanges(
		"GET", "/lang/en",
	)

	patterns := GroupPatterns(exchanges)

	key := "GET /lang/en"
	if _, ok := patterns[key]; !ok {
		t.Errorf("expected pattern %q, got keys: %v", key, patternKeys(patterns))
	}
}

func TestGroupPatterns_SinglePath(t *testing.T) {
	exchanges := makeExchanges(
		"POST", "/api/users",
	)

	patterns := GroupPatterns(exchanges)
	if len(patterns) != 1 {
		t.Errorf("expected 1 pattern, got %d", len(patterns))
	}

	key := "POST /api/users"
	if _, ok := patterns[key]; !ok {
		t.Errorf("expected pattern %q, got keys: %v", key, patternKeys(patterns))
	}
}

func TestGroupPatterns_TrailingSlash(t *testing.T) {
	exchanges := makeExchanges(
		"GET", "/api/users/",
	)

	patterns := GroupPatterns(exchanges)
	// Should handle without error
	if len(patterns) != 1 {
		t.Errorf("expected 1 pattern, got %d: %v", len(patterns), patternKeys(patterns))
	}
}

func TestGroupPatterns_RootPath(t *testing.T) {
	exchanges := makeExchanges(
		"GET", "/",
	)

	patterns := GroupPatterns(exchanges)
	if len(patterns) != 1 {
		t.Errorf("expected 1 pattern, got %d", len(patterns))
	}

	key := "GET /"
	if _, ok := patterns[key]; !ok {
		t.Errorf("expected pattern %q, got keys: %v", key, patternKeys(patterns))
	}
}

func TestGroupPatterns_MultipleMethods(t *testing.T) {
	exchanges := []capture.CapturedExchange{
		{Method: "GET", Path: "/users/123"},
		{Method: "GET", Path: "/users/456"},
		{Method: "GET", Path: "/users/789"},
		{Method: "GET", Path: "/users/101"},
		{Method: "DELETE", Path: "/users/123"},
		{Method: "DELETE", Path: "/users/456"},
		{Method: "DELETE", Path: "/users/789"},
		{Method: "DELETE", Path: "/users/101"},
	}

	patterns := GroupPatterns(exchanges)

	getKey := "GET /users/{id}"
	deleteKey := "DELETE /users/{id}"

	if _, ok := patterns[getKey]; !ok {
		t.Errorf("expected pattern %q, got keys: %v", getKey, patternKeys(patterns))
	}
	if _, ok := patterns[deleteKey]; !ok {
		t.Errorf("expected pattern %q, got keys: %v", deleteKey, patternKeys(patterns))
	}
}

func TestNormalizeSinglePath_NumericID(t *testing.T) {
	result := NormalizeSinglePath("/users/12345")
	if result != "/users/{id}" {
		t.Errorf("expected /users/{id}, got %s", result)
	}
}

func TestNormalizeSinglePath_UUID(t *testing.T) {
	result := NormalizeSinglePath("/items/550e8400-e29b-41d4-a716-446655440000")
	if result != "/items/{id}" {
		t.Errorf("expected /items/{id}, got %s", result)
	}
}

func TestNormalizeSinglePath_ShortStatic(t *testing.T) {
	result := NormalizeSinglePath("/api/v1/health")
	if result != "/api/v1/health" {
		t.Errorf("expected /api/v1/health, got %s", result)
	}
}

func TestIsLikelyID(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"123", true},    // numeric, always ID
		{"1234", true},   // numeric, 4 chars
		{"me", false},    // too short
		{"en", false},    // too short
		{"550e8400-e29b-41d4-a716-446655440000", true}, // UUID
		{"abcdef0123456789", true},                      // hex 16+
		{"health", false},                                // not an ID pattern
		{"v1", false},                                    // too short
		{"users", false},                                 // not an ID pattern
	}

	for _, tt := range tests {
		got := isLikelyID(tt.input)
		if got != tt.expected {
			t.Errorf("isLikelyID(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

// Helpers

func makeExchanges(methodPaths ...string) []capture.CapturedExchange {
	var exchanges []capture.CapturedExchange
	for i := 0; i < len(methodPaths); i += 2 {
		exchanges = append(exchanges, capture.CapturedExchange{
			Method: methodPaths[i],
			Path:   methodPaths[i+1],
		})
	}
	return exchanges
}

func patternKeys(patterns map[string][]string) []string {
	keys := make([]string, 0, len(patterns))
	for k := range patterns {
		keys = append(keys, k)
	}
	return keys
}
