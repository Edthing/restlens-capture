package storage

import (
	"database/sql"
	"fmt"

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
func OpenDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000&_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Single writer connection avoids SQLITE_BUSY in concurrent scenarios
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return db, nil
}
