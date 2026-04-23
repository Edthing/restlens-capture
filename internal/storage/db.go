package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	sqlite "modernc.org/sqlite"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS captured_exchanges (
    id TEXT PRIMARY KEY,
    timestamp TEXT NOT NULL,
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    query_string TEXT NOT NULL DEFAULT '',
    request_headers TEXT NOT NULL DEFAULT '{}',
    request_body_schema TEXT,
    request_content_type TEXT NOT NULL DEFAULT '',
    response_status INTEGER NOT NULL,
    response_headers TEXT NOT NULL DEFAULT '{}',
    response_body_schema TEXT,
    response_content_type TEXT NOT NULL DEFAULT '',
    latency_ms INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_exchanges_method_path ON captured_exchanges(method, path);
CREATE INDEX IF NOT EXISTS idx_exchanges_timestamp ON captured_exchanges(timestamp);
CREATE INDEX IF NOT EXISTS idx_exchanges_status ON captured_exchanges(response_status);
`

// OpenDB opens (or creates) the SQLite database at the given path and runs migrations.
//
// Error messages are wrapped to be actionable. The underlying driver
// (modernc.org/sqlite) reports SQLITE_CANTOPEN (code 14) with the literal text
// "out of memory", which is extremely misleading when the real problem is a
// missing parent directory or a read-only mount. We preflight the path and
// translate known SQLite codes into messages that point at the actual cause.
func OpenDB(path string) (*sql.DB, error) {
	if path == "" {
		return nil, errors.New("database path is empty (pass --db=/path/to/capture.db)")
	}

	if err := preflightDBPath(path); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000&_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf("open sqlite at %s: %w", path, err)
	}

	// Single writer connection avoids SQLITE_BUSY in concurrent scenarios
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, translateSQLiteError(path, "run migrations", err)
	}

	return db, nil
}

// preflightDBPath returns a useful error before SQLite gets involved, because
// SQLite's own errors for these cases are misleading ("out of memory") or
// generic. Covers the three cases that account for basically every failure we
// see in the wild: missing parent dir, parent-is-a-file, and unwritable dir.
func preflightDBPath(path string) error {
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}

	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("database directory does not exist: %s (create it first, or point --db at an existing directory)", dir)
		}
		return fmt.Errorf("cannot stat database directory %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("database path parent %s is a file, not a directory", dir)
	}

	// Touch a probe file to catch permission issues with a clear message
	// rather than surfacing SQLITE_CANTOPEN downstream.
	probe, err := os.CreateTemp(dir, ".restlens-perm-probe-*")
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("no write permission on database directory %s (process uid=%d; chown the directory or use a named docker volume)", dir, os.Geteuid())
		}
		// Read-only filesystem surfaces as EROFS which isn't os.ErrPermission.
		return fmt.Errorf("database directory %s is not writable: %w", dir, err)
	}
	probe.Close()
	os.Remove(probe.Name())
	return nil
}

// translateSQLiteError turns a *sqlite.Error into something that tells the
// user what's actually wrong. Only handles codes we've actually seen confuse
// people; anything else falls through with the original error attached.
func translateSQLiteError(path, stage string, err error) error {
	var se *sqlite.Error
	if errors.As(err, &se) {
		switch se.Code() {
		case 14: // SQLITE_CANTOPEN
			return fmt.Errorf("%s: SQLite cannot open %s (SQLITE_CANTOPEN=14): check that the parent directory exists, is writable by the current user, and is not on a read-only mount. Underlying driver message is misleading (%q).", stage, path, se.Error())
		case 26: // SQLITE_NOTADB
			return fmt.Errorf("%s: %s exists but is not a SQLite database (SQLITE_NOTADB=26): delete the file or point --db at a different path", stage, path)
		case 13: // SQLITE_FULL
			return fmt.Errorf("%s: disk full while writing %s (SQLITE_FULL=13)", stage, path)
		case 8: // SQLITE_READONLY
			return fmt.Errorf("%s: %s is on a read-only filesystem (SQLITE_READONLY=8)", stage, path)
		case 11: // SQLITE_CORRUPT
			return fmt.Errorf("%s: %s is corrupted (SQLITE_CORRUPT=11): delete the file and let it be recreated", stage, path)
		}
	}
	return fmt.Errorf("%s at %s: %w", stage, path, err)
}
