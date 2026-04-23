package storage

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Edthing/restlens-capture/internal/capture"
	"github.com/google/uuid"
)

func TestOpenDB_CreatesFileAndMigrates(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	// Verify file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("database file was not created")
	}

	// Verify table exists
	var tableName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='captured_exchanges'").Scan(&tableName)
	if err != nil {
		t.Fatalf("table not found: %v", err)
	}
	if tableName != "captured_exchanges" {
		t.Errorf("expected table 'captured_exchanges', got %q", tableName)
	}
}

func TestOpenDB_InvalidPath(t *testing.T) {
	_, err := OpenDB("/nonexistent/deep/path/test.db")
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
	// Must NOT be the misleading modernc message. Must point at the real cause.
	msg := err.Error()
	if !strings.Contains(msg, "does not exist") {
		t.Errorf("expected actionable 'does not exist' message, got: %v", err)
	}
	if strings.Contains(msg, "out of memory") {
		t.Errorf("error message still contains the misleading 'out of memory' phrase from modernc/sqlite: %v", err)
	}
}

func TestOpenDB_EmptyPath(t *testing.T) {
	_, err := OpenDB("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected message mentioning empty path, got: %v", err)
	}
}

func TestOpenDB_ParentIsFile(t *testing.T) {
	dir := t.TempDir()
	// Create a regular file, then try to use it as if it were a parent directory.
	fakeParent := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(fakeParent, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := OpenDB(filepath.Join(fakeParent, "capture.db"))
	if err == nil {
		t.Fatal("expected error when parent is a file")
	}
	// Either the stat fails (ENOTDIR) or our explicit check fires — both are fine,
	// as long as the message isn't the misleading SQLite one.
	if strings.Contains(err.Error(), "out of memory") {
		t.Errorf("error leaked misleading 'out of memory' phrase: %v", err)
	}
}

func TestOpenDB_UnwritableDirectory(t *testing.T) {
	// Can't reliably test permission denial as root — chmod is a no-op for root.
	if os.Geteuid() == 0 {
		t.Skip("running as root; chmod-based permission test would be meaningless")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) }) // so t.TempDir cleanup works

	_, err := OpenDB(filepath.Join(dir, "capture.db"))
	if err == nil {
		t.Fatal("expected permission error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "write permission") && !strings.Contains(msg, "not writable") {
		t.Errorf("expected a permission-oriented message, got: %v", err)
	}
	if strings.Contains(msg, "out of memory") {
		t.Errorf("error leaked misleading 'out of memory' phrase: %v", err)
	}
}

func TestOpenDB_NotADatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "bogus.db")
	// Write something that's definitely not a SQLite header.
	if err := os.WriteFile(dbPath, []byte("this is not a SQLite database, just text\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := OpenDB(dbPath)
	if err == nil {
		t.Fatal("expected error when file is not a database")
	}
	msg := err.Error()
	if !strings.Contains(msg, "not a SQLite database") {
		t.Errorf("expected 'not a SQLite database' message, got: %v", err)
	}
}

func TestInsertAndLoadExchange_RoundTrip(t *testing.T) {
	db := openTestDB(t)

	ex := makeExchange("GET", "/api/users", 200, 45)
	if err := InsertExchange(db, ex); err != nil {
		t.Fatalf("InsertExchange: %v", err)
	}

	exchanges, err := LoadAllExchanges(db)
	if err != nil {
		t.Fatalf("LoadAllExchanges: %v", err)
	}

	if len(exchanges) != 1 {
		t.Fatalf("expected 1 exchange, got %d", len(exchanges))
	}

	got := exchanges[0]
	if got.ID != ex.ID {
		t.Errorf("ID mismatch: %s != %s", got.ID, ex.ID)
	}
	if got.Method != ex.Method {
		t.Errorf("Method mismatch: %s != %s", got.Method, ex.Method)
	}
	if got.Path != ex.Path {
		t.Errorf("Path mismatch: %s != %s", got.Path, ex.Path)
	}
	if got.ResponseStatus != ex.ResponseStatus {
		t.Errorf("Status mismatch: %d != %d", got.ResponseStatus, ex.ResponseStatus)
	}
	if got.LatencyMs != ex.LatencyMs {
		t.Errorf("Latency mismatch: %d != %d", got.LatencyMs, ex.LatencyMs)
	}
	if got.RequestContentType != ex.RequestContentType {
		t.Errorf("RequestContentType mismatch: %s != %s", got.RequestContentType, ex.RequestContentType)
	}
}

func TestInsertAndLoad_WithSchemas(t *testing.T) {
	db := openTestDB(t)

	ex := makeExchange("POST", "/api/users", 201, 100)
	ex.RequestBodySchema = json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`)
	ex.ResponseBodySchema = json.RawMessage(`{"type":"object","properties":{"id":{"type":"integer"}}}`)

	if err := InsertExchange(db, ex); err != nil {
		t.Fatalf("InsertExchange: %v", err)
	}

	exchanges, err := LoadAllExchanges(db)
	if err != nil {
		t.Fatalf("LoadAllExchanges: %v", err)
	}

	got := exchanges[0]
	if got.RequestBodySchema == nil {
		t.Fatal("expected non-nil RequestBodySchema")
	}
	if got.ResponseBodySchema == nil {
		t.Fatal("expected non-nil ResponseBodySchema")
	}
}

func TestBulkInsert_1000Exchanges(t *testing.T) {
	db := openTestDB(t)

	batch := make([]*capture.CapturedExchange, 1000)
	for i := 0; i < 1000; i++ {
		batch[i] = makeExchange("GET", "/api/items", 200, int64(i))
	}

	if err := InsertExchangeBatch(db, batch); err != nil {
		t.Fatalf("InsertExchangeBatch: %v", err)
	}

	count, err := CountExchanges(db)
	if err != nil {
		t.Fatalf("CountExchanges: %v", err)
	}
	if count != 1000 {
		t.Errorf("expected 1000, got %d", count)
	}

	exchanges, err := LoadAllExchanges(db)
	if err != nil {
		t.Fatalf("LoadAllExchanges: %v", err)
	}
	if len(exchanges) != 1000 {
		t.Errorf("expected 1000 exchanges, got %d", len(exchanges))
	}
}

func TestLoadAllExchanges_EmptyDB(t *testing.T) {
	db := openTestDB(t)

	exchanges, err := LoadAllExchanges(db)
	if err != nil {
		t.Fatalf("LoadAllExchanges: %v", err)
	}
	if len(exchanges) != 0 {
		t.Errorf("expected 0 exchanges, got %d", len(exchanges))
	}
}

func TestLoadAllExchanges_OrderedByTimestamp(t *testing.T) {
	db := openTestDB(t)

	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		ex := makeExchange("GET", "/api/test", 200, 10)
		ex.Timestamp = now.Add(time.Duration(4-i) * time.Minute) // Insert in reverse order
		InsertExchange(db, ex)
	}

	exchanges, err := LoadAllExchanges(db)
	if err != nil {
		t.Fatalf("LoadAllExchanges: %v", err)
	}

	for i := 1; i < len(exchanges); i++ {
		if exchanges[i].Timestamp.Before(exchanges[i-1].Timestamp) {
			t.Errorf("exchanges not ordered by timestamp: %v before %v",
				exchanges[i].Timestamp, exchanges[i-1].Timestamp)
		}
	}
}

func TestConcurrentWrites(t *testing.T) {
	db := openTestDB(t)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				ex := makeExchange("GET", "/api/concurrent", 200, int64(n*10+j))
				if err := InsertExchange(db, ex); err != nil {
					t.Errorf("concurrent insert failed: %v", err)
				}
			}
		}(i)
	}
	wg.Wait()

	count, err := CountExchanges(db)
	if err != nil {
		t.Fatalf("CountExchanges: %v", err)
	}
	if count != 100 {
		t.Errorf("expected 100, got %d", count)
	}
}

func TestCountExchanges(t *testing.T) {
	db := openTestDB(t)

	count, err := CountExchanges(db)
	if err != nil {
		t.Fatalf("CountExchanges: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	InsertExchange(db, makeExchange("GET", "/a", 200, 10))
	InsertExchange(db, makeExchange("POST", "/b", 201, 20))

	count, err = CountExchanges(db)
	if err != nil {
		t.Fatalf("CountExchanges: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestInsertExchange_HeadersSerialization(t *testing.T) {
	db := openTestDB(t)

	ex := makeExchange("GET", "/api/test", 200, 10)
	ex.RequestHeaders = map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer token123",
	}
	ex.ResponseHeaders = map[string]string{
		"Content-Type": "application/json",
		"X-Request-Id": "abc-123",
	}

	InsertExchange(db, ex)

	exchanges, _ := LoadAllExchanges(db)
	got := exchanges[0]

	if got.RequestHeaders["Content-Type"] != "application/json" {
		t.Errorf("request header mismatch: %v", got.RequestHeaders)
	}
	if got.ResponseHeaders["X-Request-Id"] != "abc-123" {
		t.Errorf("response header mismatch: %v", got.ResponseHeaders)
	}
}

// Helpers

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func makeExchange(method, path string, status int, latencyMs int64) *capture.CapturedExchange {
	return &capture.CapturedExchange{
		ID:                  uuid.New().String(),
		Timestamp:           time.Now().UTC(),
		Method:              method,
		Path:                path,
		RequestHeaders:      map[string]string{"Content-Type": "application/json"},
		RequestContentType:  "application/json",
		ResponseStatus:      status,
		ResponseHeaders:     map[string]string{"Content-Type": "application/json"},
		ResponseContentType: "application/json",
		LatencyMs:           latencyMs,
	}
}
