package proxy

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Edthing/restlens-capture/internal/storage"
)

func TestProxy_ForwardsGET(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer backend.Close()

	proxy, db := startTestProxy(t, backend.URL)
	defer db.Close()

	resp, err := http.Get(proxy.URL + "/api/health")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"status":"ok"}` {
		t.Errorf("unexpected body: %s", string(body))
	}

	// Wait for capture to flush
	time.Sleep(700 * time.Millisecond)

	exchanges, _ := storage.LoadAllExchanges(db)
	if len(exchanges) < 1 {
		t.Fatal("expected at least 1 captured exchange")
	}

	ex := exchanges[0]
	if ex.Method != "GET" {
		t.Errorf("expected GET, got %s", ex.Method)
	}
	if ex.Path != "/api/health" {
		t.Errorf("expected /api/health, got %s", ex.Path)
	}
	if ex.ResponseStatus != 200 {
		t.Errorf("expected status 200, got %d", ex.ResponseStatus)
	}
	if ex.LatencyMs < 0 {
		t.Errorf("expected latency >= 0, got %d", ex.LatencyMs)
	}
}

func TestProxy_ForwardsPOSTWithJSONBody(t *testing.T) {
	var receivedBody string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		w.Write([]byte(`{"id":1}`))
	}))
	defer backend.Close()

	proxy, _ := startTestProxy(t, backend.URL)

	reqBody := `{"name":"test","email":"test@example.com"}`
	resp, err := http.Post(proxy.URL+"/api/users", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}

	if receivedBody != reqBody {
		t.Errorf("backend received different body: %s", receivedBody)
	}
}

func TestProxy_ForwardsHeaders(t *testing.T) {
	var receivedAuth string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("X-Custom", "response-header")
		w.WriteHeader(200)
	}))
	defer backend.Close()

	proxy, _ := startTestProxy(t, backend.URL)

	req, _ := http.NewRequest("GET", proxy.URL+"/api/test", nil)
	req.Header.Set("Authorization", "Bearer my-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if receivedAuth != "Bearer my-token" {
		t.Errorf("backend didn't receive auth header: %s", receivedAuth)
	}

	if resp.Header.Get("X-Custom") != "response-header" {
		t.Errorf("response header not forwarded: %v", resp.Header)
	}
}

func TestProxy_UnknownEndpoint500_NotCaptured(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal error"))
	}))
	defer backend.Close()

	proxy, db := startTestProxy(t, backend.URL)
	defer db.Close()

	resp, err := http.Get(proxy.URL + "/api/never-seen-before")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 500 {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}

	// Unknown endpoint + non-2xx = NOT captured
	time.Sleep(700 * time.Millisecond)
	exchanges, _ := storage.LoadAllExchanges(db)
	if len(exchanges) != 0 {
		t.Errorf("expected 0 captured exchanges for unknown endpoint, got %d", len(exchanges))
	}
}

func TestProxy_KnownEndpoint404_Captured(t *testing.T) {
	callCount := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write([]byte(`{"id":1}`))
		} else {
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"not found"}`))
		}
	}))
	defer backend.Close()

	proxy, db := startTestProxy(t, backend.URL)
	defer db.Close()

	// First request: 200 — establishes /api/users/{id} as known
	resp, _ := http.Get(proxy.URL + "/api/users/1001")
	resp.Body.Close()

	// Second request: 404 — same pattern, should be captured
	resp, _ = http.Get(proxy.URL + "/api/users/1002")
	resp.Body.Close()

	time.Sleep(700 * time.Millisecond)
	exchanges, _ := storage.LoadAllExchanges(db)

	if len(exchanges) != 2 {
		t.Errorf("expected 2 captured (200 + 404 on known endpoint), got %d", len(exchanges))
		for _, ex := range exchanges {
			t.Logf("  captured: %s %s -> %d", ex.Method, ex.Path, ex.ResponseStatus)
		}
	}
}

func TestProxy_CapturePolicy(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/real":
			w.WriteHeader(200)
			w.Write([]byte(`{"ok":true}`))
		case "/api/real-error":
			// Same normalized pattern as /api/real won't match, different path
			w.WriteHeader(500)
		case "/api/spam":
			w.WriteHeader(404)
		case "/api/created":
			w.WriteHeader(201)
		}
	}))
	defer backend.Close()

	proxy, db := startTestProxy(t, backend.URL)
	defer db.Close()

	// /api/real -> 200: captured, pattern becomes known
	resp, _ := http.Get(proxy.URL + "/api/real")
	resp.Body.Close()

	// /api/spam -> 404: NOT captured (unknown pattern, no prior 2xx)
	resp, _ = http.Get(proxy.URL + "/api/spam")
	resp.Body.Close()

	// /api/created -> 201: captured (2xx)
	resp, _ = http.Get(proxy.URL + "/api/created")
	resp.Body.Close()

	time.Sleep(700 * time.Millisecond)
	exchanges, _ := storage.LoadAllExchanges(db)

	// /api/real (200) + /api/created (201) = 2; /api/spam (404, unknown) = skipped
	if len(exchanges) != 2 {
		t.Errorf("expected 2 captured, got %d", len(exchanges))
		for _, ex := range exchanges {
			t.Logf("  captured: %s %s -> %d", ex.Method, ex.Path, ex.ResponseStatus)
		}
	}
}

func TestProxy_HandlesNonJSON(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("hello plain text"))
	}))
	defer backend.Close()

	proxy, db := startTestProxy(t, backend.URL)
	defer db.Close()

	resp, err := http.Get(proxy.URL + "/api/text")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello plain text" {
		t.Errorf("unexpected body: %s", body)
	}

	time.Sleep(700 * time.Millisecond)
	exchanges, _ := storage.LoadAllExchanges(db)
	if len(exchanges) < 1 {
		t.Fatal("expected captured exchange")
	}
	if exchanges[0].ResponseBodySchema != nil {
		t.Error("expected nil response body schema for non-JSON")
	}
}

func TestProxy_Healthz(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("/healthz should not reach backend")
	}))
	defer backend.Close()

	proxy, _ := startTestProxy(t, backend.URL)

	resp, err := http.Get(proxy.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("expected 'ok', got %q", string(body))
	}
}

func TestProxy_CapturesSchemas(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"users":[{"id":1,"name":"Alice"}],"total":1}`))
	}))
	defer backend.Close()

	proxy, db := startTestProxy(t, backend.URL)
	defer db.Close()

	reqBody := `{"query":"active","limit":10}`
	resp, _ := http.Post(proxy.URL+"/api/search", "application/json", strings.NewReader(reqBody))
	resp.Body.Close()

	time.Sleep(700 * time.Millisecond)

	exchanges, _ := storage.LoadAllExchanges(db)
	if len(exchanges) < 1 {
		t.Fatal("expected captured exchange")
	}

	ex := exchanges[0]

	// Check request schema captured
	if ex.RequestBodySchema == nil {
		t.Fatal("expected request body schema")
	}
	var reqSchema map[string]any
	json.Unmarshal(ex.RequestBodySchema, &reqSchema)
	if reqSchema["type"] != "object" {
		t.Errorf("expected object type for request schema, got %v", reqSchema["type"])
	}

	// Check response schema captured
	if ex.ResponseBodySchema == nil {
		t.Fatal("expected response body schema")
	}
	var respSchema map[string]any
	json.Unmarshal(ex.ResponseBodySchema, &respSchema)
	if respSchema["type"] != "object" {
		t.Errorf("expected object type for response schema, got %v", respSchema["type"])
	}

	// Verify no actual values in schema
	schemaStr := string(ex.ResponseBodySchema)
	if strings.Contains(schemaStr, "Alice") {
		t.Error("schema contains actual value 'Alice'")
	}
}

func TestProxy_ConcurrentRequests(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer backend.Close()

	proxy, db := startTestProxy(t, backend.URL)
	defer db.Close()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			resp, err := http.Get(fmt.Sprintf("%s/api/item/%d", proxy.URL, n))
			if err != nil {
				t.Errorf("request %d failed: %v", n, err)
				return
			}
			resp.Body.Close()
		}(i)
	}
	wg.Wait()

	time.Sleep(1 * time.Second)

	count, err := storage.CountExchanges(db)
	if err != nil {
		t.Fatalf("CountExchanges: %v", err)
	}
	if count != 50 {
		t.Errorf("expected 50 captured exchanges, got %d", count)
	}
}

func TestProxy_QueryStringPreserved(t *testing.T) {
	var receivedQuery string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		w.WriteHeader(200)
	}))
	defer backend.Close()

	proxy, db := startTestProxy(t, backend.URL)
	defer db.Close()

	resp, _ := http.Get(proxy.URL + "/api/search?q=test&page=2")
	resp.Body.Close()

	if receivedQuery != "q=test&page=2" {
		t.Errorf("query not forwarded: %s", receivedQuery)
	}

	time.Sleep(700 * time.Millisecond)
	exchanges, _ := storage.LoadAllExchanges(db)
	if len(exchanges) < 1 {
		t.Fatal("expected captured exchange")
	}
	if exchanges[0].QueryString != "q=test&page=2" {
		t.Errorf("query not captured: %s", exchanges[0].QueryString)
	}
}

// Helpers

func startTestProxy(t *testing.T, targetURL string) (*httptest.Server, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}

	ct := NewCaptureTransport(db, true, true)
	t.Cleanup(func() { ct.Close() })

	targetParsed, _ := url.Parse(targetURL)
	rp := httputil.NewSingleHostReverseProxy(targetParsed)
	rp.Transport = ct

	originalDirector := rp.Director
	rp.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = targetParsed.Host
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	})
	mux.Handle("/", rp)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server, db
}
