package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Edthing/restlens-capture/internal/export"
	"github.com/Edthing/restlens-capture/internal/proxy"
	"github.com/Edthing/restlens-capture/internal/report"
	"github.com/Edthing/restlens-capture/internal/storage"
	"gopkg.in/yaml.v3"
)

func TestE2E_CaptureAndReport(t *testing.T) {
	// 1. Start a mock backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/users/"):
			w.WriteHeader(200)
			w.Write([]byte(`{"id":1,"name":"Alice","email":"alice@example.com"}`))

		case r.Method == "GET" && r.URL.Path == "/api/users":
			w.WriteHeader(200)
			w.Write([]byte(`[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"}]`))

		case r.Method == "POST" && r.URL.Path == "/api/users":
			body, _ := io.ReadAll(r.Body)
			_ = body
			w.WriteHeader(201)
			w.Write([]byte(`{"id":3,"name":"Charlie"}`))

		case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/api/users/"):
			w.WriteHeader(200)
			w.Write([]byte(`{"id":1,"name":"Updated"}`))

		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/api/users/"):
			w.WriteHeader(204)

		case r.Method == "GET" && r.URL.Path == "/api/health":
			w.WriteHeader(200)
			w.Write([]byte(`{"status":"healthy"}`))

		default:
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"not found"}`))
		}
	}))
	defer backend.Close()

	// 2. Set up proxy with SQLite capture
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "e2e-test.db")
	db, err := storage.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	ct := proxy.NewCaptureTransport(db, true, true)
	defer ct.Close()

	targetURL, _ := url.Parse(backend.URL)
	rp := httputil.NewSingleHostReverseProxy(targetURL)
	rp.Transport = ct
	originalDirector := rp.Director
	rp.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = targetURL.Host
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	})
	mux.Handle("/", rp)

	proxyServer := httptest.NewServer(mux)
	defer proxyServer.Close()

	// 3. Send 10 varied requests through the proxy
	requests := []struct {
		method string
		path   string
		body   string
	}{
		{"GET", "/api/users", ""},
		{"GET", "/api/users/1001", ""},
		{"GET", "/api/users/1002", ""},
		{"GET", "/api/users/1003", ""},
		{"POST", "/api/users", `{"name":"Dave","email":"dave@test.com"}`},
		{"PUT", "/api/users/1001", `{"name":"Updated Dave"}`},
		{"DELETE", "/api/users/1002", ""},
		{"GET", "/api/health", ""},
		{"GET", "/api/users/1004", ""},
		{"GET", "/api/nonexistent", ""},
	}

	client := &http.Client{Timeout: 5 * time.Second}
	for _, r := range requests {
		var bodyReader io.Reader
		if r.body != "" {
			bodyReader = strings.NewReader(r.body)
		}
		req, _ := http.NewRequest(r.method, proxyServer.URL+r.path, bodyReader)
		if r.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request %s %s failed: %v", r.method, r.path, err)
		}
		resp.Body.Close()
	}

	// 4. Wait for captures to flush
	time.Sleep(1 * time.Second)

	// 5. Verify DB has only 2xx exchanges (9 of 10 — /api/nonexistent returns 404)
	count, err := storage.CountExchanges(db)
	if err != nil {
		t.Fatalf("CountExchanges: %v", err)
	}
	if count != 9 {
		t.Errorf("expected 9 captured exchanges (2xx only), got %d", count)
	}

	exchanges, err := storage.LoadAllExchanges(db)
	if err != nil {
		t.Fatalf("LoadAllExchanges: %v", err)
	}

	// Verify no non-2xx responses were captured
	for _, ex := range exchanges {
		if ex.ResponseStatus < 200 || ex.ResponseStatus >= 300 {
			t.Errorf("non-2xx exchange captured: %s %s -> %d", ex.Method, ex.Path, ex.ResponseStatus)
		}
	}

	// 6. Generate report
	patterns := export.GroupPatterns(exchanges)
	summary := report.BuildSummary(exchanges, patterns)

	if summary.TotalHits != 9 {
		t.Errorf("report total hits: expected 9, got %d", summary.TotalHits)
	}

	if len(summary.Endpoints) == 0 {
		t.Fatal("expected at least 1 endpoint in report")
	}

	// Terminal output should work without error
	var termBuf bytes.Buffer
	report.RenderTerminal(summary, &termBuf)
	if termBuf.Len() == 0 {
		t.Error("expected non-empty terminal output")
	}

	// JSON output should be valid
	var jsonBuf bytes.Buffer
	if err := report.RenderJSON(summary, &jsonBuf); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	var parsedSummary report.TrafficSummary
	if err := json.Unmarshal(jsonBuf.Bytes(), &parsedSummary); err != nil {
		t.Fatalf("invalid JSON report: %v", err)
	}

	// 7. Generate OpenAPI spec
	spec := export.GenerateOpenAPI(exchanges)
	if spec.OpenAPI != "3.1.0" {
		t.Errorf("expected OpenAPI 3.1.0, got %s", spec.OpenAPI)
	}

	var yamlBuf bytes.Buffer
	if err := export.WriteOpenAPIYAML(spec, &yamlBuf); err != nil {
		t.Fatalf("WriteOpenAPIYAML: %v", err)
	}

	// Verify valid YAML
	var parsedYAML map[string]any
	if err := yaml.Unmarshal(yamlBuf.Bytes(), &parsedYAML); err != nil {
		t.Fatalf("invalid OpenAPI YAML: %v", err)
	}

	// Verify paths exist
	paths, ok := parsedYAML["paths"].(map[string]any)
	if !ok {
		t.Fatal("expected paths in OpenAPI spec")
	}
	if len(paths) == 0 {
		t.Error("expected at least 1 path in OpenAPI spec")
	}

	// 8. Verify SQLite file exists on disk
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("SQLite database file does not exist")
	}

	// Print summary for debugging
	fmt.Printf("\nE2E Test Summary:\n")
	fmt.Printf("  Captured: %d exchanges\n", count)
	fmt.Printf("  Endpoints: %d\n", len(summary.Endpoints))
	fmt.Printf("  OpenAPI paths: %d\n", len(paths))
	for p := range paths {
		fmt.Printf("    - %s\n", p)
	}
}
