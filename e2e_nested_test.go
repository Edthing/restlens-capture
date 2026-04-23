package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Edthing/restlens-capture/internal/export"
	"github.com/Edthing/restlens-capture/internal/proxy"
	"github.com/Edthing/restlens-capture/internal/storage"
)

// TestE2E_NestedCRUD exercises restlens-capture against a realistic backend
// with nested resources and asserts the exact set of OpenAPI paths produced.
//
// This is a regression test for the "one operation on export" bug: several
// route families here intentionally share (method, depth) with other families
// but differ in their static segments — the old grouper would have merged them
// into a single pattern. A weaker assertion like "len(paths) > 0" would miss
// that, so this test pins the exact expected path set.
func TestE2E_NestedCRUD(t *testing.T) {
	backend := httptest.NewServer(nestedBackend())
	defer backend.Close()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "e2e-nested.db")
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
	proxyServer := httptest.NewServer(rp)
	defer proxyServer.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	send := func(method, path, body string) {
		t.Helper()
		var r io.Reader
		if body != "" {
			r = strings.NewReader(body)
		}
		req, _ := http.NewRequest(method, proxyServer.URL+path, r)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request %s %s: %v", method, path, err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	// Realistic session: auth, flat collections, nested 1-level, nested 2-level.
	send("POST", "/api/auth/login", `{"email":"a@x","password":"p"}`)
	send("GET", "/api/me", "")
	send("GET", "/api/health", "")
	send("GET", "/api/settings", "")
	send("GET", "/api/feed", "")

	// users CRUD
	send("GET", "/api/users", "")
	for _, id := range []string{"1", "2", "3"} {
		send("GET", "/api/users/"+id, "")
	}
	send("POST", "/api/users", `{"name":"Dave","email":"d@x"}`)
	send("POST", "/api/users", `{"name":"Eve","email":"e@x"}`)
	send("PUT", "/api/users/4", `{"name":"Dave v2"}`)
	send("DELETE", "/api/users/5", "")

	// static-under-dynamic: /api/users/{id}/posts
	for _, id := range []string{"1", "2", "3"} {
		send("GET", "/api/users/"+id+"/posts", "")
	}

	// posts CRUD
	send("GET", "/api/posts", "")
	for _, id := range []string{"1", "2", "3"} {
		send("GET", "/api/posts/"+id, "")
	}
	send("POST", "/api/posts", `{"userId":1,"title":"t","body":"b"}`)
	send("PUT", "/api/posts/4", `{"title":"t2"}`)
	send("DELETE", "/api/posts/4", "")

	// nested: comments / likes / tags all share (method, depth) — must stay separate
	for _, postID := range []string{"1", "2", "3"} {
		send("GET", "/api/posts/"+postID+"/comments", "")
		send("GET", "/api/posts/"+postID+"/likes", "")
		send("GET", "/api/posts/"+postID+"/tags", "")
	}
	send("POST", "/api/posts/1/comments", `{"author":"x","body":"y"}`)
	send("POST", "/api/posts/2/likes", `{"userId":4}`)
	send("POST", "/api/posts/3/tags", `{"name":"featured"}`)

	// 2-level nested items — three families that would have merged under the old bug
	send("GET", "/api/posts/1/comments/10", "")
	send("GET", "/api/posts/2/comments/11", "")
	send("PUT", "/api/posts/1/comments/10", `{"body":"edit"}`)
	send("DELETE", "/api/posts/1/comments/10", "")
	send("DELETE", "/api/posts/1/likes/20", "")
	send("DELETE", "/api/posts/1/tags/30", "")

	send("POST", "/api/auth/logout", `{}`)

	// give the async writer time to flush
	time.Sleep(1 * time.Second)

	exchanges, err := storage.LoadAllExchanges(db)
	if err != nil {
		t.Fatalf("LoadAllExchanges: %v", err)
	}
	if len(exchanges) == 0 {
		t.Fatal("no exchanges captured")
	}

	spec := export.GenerateOpenAPI(exchanges)
	got := make([]string, 0, len(spec.Paths))
	for p := range spec.Paths {
		got = append(got, p)
	}
	sort.Strings(got)

	want := []string{
		"/api/auth/login",
		"/api/auth/logout",
		"/api/feed",
		"/api/health",
		"/api/me",
		"/api/posts",
		"/api/posts/{id}",
		"/api/posts/{id}/comments",
		"/api/posts/{id}/comments/{id}",
		"/api/posts/{id}/likes",
		"/api/posts/{id}/likes/{id}",
		"/api/posts/{id}/tags",
		"/api/posts/{id}/tags/{id}",
		"/api/settings",
		"/api/users",
		"/api/users/{id}",
		"/api/users/{id}/posts",
	}

	if !equalStringSlices(got, want) {
		t.Errorf("path set mismatch\n got:  %v\n want: %v", got, want)
	}

	// Nail the specific regression: three route families that share method+depth
	// but differ only by static leaf segment MUST be separate operations.
	mustHavePath(t, spec, "/api/posts/{id}/comments")
	mustHavePath(t, spec, "/api/posts/{id}/likes")
	mustHavePath(t, spec, "/api/posts/{id}/tags")
	mustHavePath(t, spec, "/api/posts/{id}/comments/{id}")
	mustHavePath(t, spec, "/api/posts/{id}/likes/{id}")
	mustHavePath(t, spec, "/api/posts/{id}/tags/{id}")

	// And the other classic: flat static endpoints sharing (method, depth).
	mustHavePath(t, spec, "/api/me")
	mustHavePath(t, spec, "/api/feed")
	mustHavePath(t, spec, "/api/health")
	mustHavePath(t, spec, "/api/settings")
	mustHavePath(t, spec, "/api/auth/login")
	mustHavePath(t, spec, "/api/auth/logout")

	// Method coverage: /api/posts/{id} should have GET + PUT + DELETE.
	ops := spec.Paths["/api/posts/{id}"]
	for _, m := range []string{"get", "put", "delete"} {
		if _, ok := ops[m]; !ok {
			t.Errorf("expected %s /api/posts/{id}, got ops %v", m, opKeys(ops))
		}
	}
}

// TestE2E_NestedCRUD_BugReproduction asserts the precise failure mode the
// original user hit: if the grouper ever regresses to merging distinct static
// routes at the same depth, the OpenAPI export collapses to a single path.
// This is the test that *would have* caught the original bug.
func TestE2E_NestedCRUD_NoSilentMerging(t *testing.T) {
	backend := httptest.NewServer(nestedBackend())
	defer backend.Close()

	dir := t.TempDir()
	db, err := storage.OpenDB(filepath.Join(dir, "merge.db"))
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
	rp.Director = func(req *http.Request) { originalDirector(req); req.Host = targetURL.Host }
	proxyServer := httptest.NewServer(rp)
	defer proxyServer.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	for _, p := range []string{"/api/me", "/api/feed", "/api/health", "/api/settings"} {
		resp, err := client.Get(proxyServer.URL + p)
		if err != nil {
			t.Fatalf("GET %s: %v", p, err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	time.Sleep(500 * time.Millisecond)

	exchanges, err := storage.LoadAllExchanges(db)
	if err != nil {
		t.Fatalf("LoadAllExchanges: %v", err)
	}
	spec := export.GenerateOpenAPI(exchanges)

	if len(spec.Paths) != 4 {
		keys := make([]string, 0, len(spec.Paths))
		for p := range spec.Paths {
			keys = append(keys, p)
		}
		sort.Strings(keys)
		t.Fatalf("expected 4 distinct paths after hitting 4 distinct static endpoints, got %d: %v", len(spec.Paths), keys)
	}
}

// nestedBackend returns an http.Handler implementing a minimal CRUD API with
// nested resources — enough route shape to exercise the path grouper.
func nestedBackend() http.Handler {
	mux := http.NewServeMux()

	okJSON := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write([]byte(body))
		}
	}
	createdJSON := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// drain request body so schema inference captures it
			io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(201)
			w.Write([]byte(body))
		}
	}
	noContent := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }

	// auth / session
	mux.HandleFunc("POST /api/auth/login", createdJSON(`{"token":"eyJ","userId":1}`))
	mux.HandleFunc("POST /api/auth/logout", okJSON(`{"ok":true}`))
	mux.HandleFunc("GET /api/me", okJSON(`{"id":1,"name":"Alice","email":"a@x"}`))
	mux.HandleFunc("GET /api/health", okJSON(`{"status":"ok"}`))
	mux.HandleFunc("GET /api/settings", okJSON(`{"theme":"dark","locale":"en"}`))
	mux.HandleFunc("GET /api/feed", okJSON(`[{"id":1,"title":"t","body":"b","userId":1}]`))

	// users
	mux.HandleFunc("GET /api/users", okJSON(`[{"id":1,"name":"Alice","email":"a@x"}]`))
	mux.HandleFunc("POST /api/users", createdJSON(`{"id":4,"name":"Dave","email":"d@x"}`))
	mux.HandleFunc("GET /api/users/{id}", okJSON(`{"id":1,"name":"Alice","email":"a@x"}`))
	mux.HandleFunc("PUT /api/users/{id}", okJSON(`{"id":1,"name":"Updated","email":"a@x"}`))
	mux.HandleFunc("DELETE /api/users/{id}", noContent)
	mux.HandleFunc("GET /api/users/{id}/posts", okJSON(`[{"id":1,"userId":1,"title":"t","body":"b"}]`))

	// posts
	mux.HandleFunc("GET /api/posts", okJSON(`[{"id":1,"userId":1,"title":"t","body":"b"}]`))
	mux.HandleFunc("POST /api/posts", createdJSON(`{"id":4,"userId":1,"title":"t","body":"b"}`))
	mux.HandleFunc("GET /api/posts/{id}", okJSON(`{"id":1,"userId":1,"title":"t","body":"b"}`))
	mux.HandleFunc("PUT /api/posts/{id}", okJSON(`{"id":1,"userId":1,"title":"t2","body":"b"}`))
	mux.HandleFunc("DELETE /api/posts/{id}", noContent)

	// nested: comments
	mux.HandleFunc("GET /api/posts/{id}/comments", okJSON(`[{"id":10,"postId":1,"author":"x","body":"y"}]`))
	mux.HandleFunc("POST /api/posts/{id}/comments", createdJSON(`{"id":10,"postId":1,"author":"x","body":"y"}`))
	mux.HandleFunc("GET /api/posts/{id}/comments/{commentID}", okJSON(`{"id":10,"postId":1,"author":"x","body":"y"}`))
	mux.HandleFunc("PUT /api/posts/{id}/comments/{commentID}", okJSON(`{"id":10,"postId":1,"author":"x","body":"edit"}`))
	mux.HandleFunc("DELETE /api/posts/{id}/comments/{commentID}", noContent)

	// nested: likes
	mux.HandleFunc("GET /api/posts/{id}/likes", okJSON(`[{"id":20,"postId":1,"userId":2}]`))
	mux.HandleFunc("POST /api/posts/{id}/likes", createdJSON(`{"id":20,"postId":1,"userId":2}`))
	mux.HandleFunc("DELETE /api/posts/{id}/likes/{likeID}", noContent)

	// nested: tags
	mux.HandleFunc("GET /api/posts/{id}/tags", okJSON(`[{"id":30,"postId":1,"name":"featured"}]`))
	mux.HandleFunc("POST /api/posts/{id}/tags", createdJSON(`{"id":30,"postId":1,"name":"featured"}`))
	mux.HandleFunc("DELETE /api/posts/{id}/tags/{tagID}", noContent)

	return mux
}

func mustHavePath(t *testing.T, spec *export.OpenAPISpec, path string) {
	t.Helper()
	if _, ok := spec.Paths[path]; !ok {
		keys := make([]string, 0, len(spec.Paths))
		for p := range spec.Paths {
			keys = append(keys, p)
		}
		sort.Strings(keys)
		t.Errorf("missing path %q in spec. got: %v", path, keys)
	}
}

func opKeys(ops map[string]export.OpItem) []string {
	keys := make([]string, 0, len(ops))
	for k := range ops {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
