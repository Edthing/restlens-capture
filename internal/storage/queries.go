package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Edthing/restlens-capture/internal/capture"
)

// InsertExchange inserts a single captured exchange into the database.
func InsertExchange(db *sql.DB, ex *capture.CapturedExchange) error {
	reqHeaders, err := json.Marshal(ex.RequestHeaders)
	if err != nil {
		return fmt.Errorf("marshal request headers: %w", err)
	}
	respHeaders, err := json.Marshal(ex.ResponseHeaders)
	if err != nil {
		return fmt.Errorf("marshal response headers: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO captured_exchanges (
			id, timestamp, method, path, query_string,
			request_headers, request_body_schema, request_content_type,
			response_status, response_headers, response_body_schema, response_content_type,
			latency_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ex.ID,
		ex.Timestamp.UTC().Format(time.RFC3339Nano),
		ex.Method,
		ex.Path,
		ex.QueryString,
		string(reqHeaders),
		nullableJSON(ex.RequestBodySchema),
		ex.RequestContentType,
		ex.ResponseStatus,
		string(respHeaders),
		nullableJSON(ex.ResponseBodySchema),
		ex.ResponseContentType,
		ex.LatencyMs,
	)
	return err
}

// InsertExchangeBatch inserts multiple exchanges in a single transaction.
func InsertExchangeBatch(db *sql.DB, exchanges []*capture.CapturedExchange) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO captured_exchanges (
			id, timestamp, method, path, query_string,
			request_headers, request_body_schema, request_content_type,
			response_status, response_headers, response_body_schema, response_content_type,
			latency_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, ex := range exchanges {
		reqHeaders, _ := json.Marshal(ex.RequestHeaders)
		respHeaders, _ := json.Marshal(ex.ResponseHeaders)

		_, err = stmt.Exec(
			ex.ID,
			ex.Timestamp.UTC().Format(time.RFC3339Nano),
			ex.Method,
			ex.Path,
			ex.QueryString,
			string(reqHeaders),
			nullableJSON(ex.RequestBodySchema),
			ex.RequestContentType,
			ex.ResponseStatus,
			string(respHeaders),
			nullableJSON(ex.ResponseBodySchema),
			ex.ResponseContentType,
			ex.LatencyMs,
		)
		if err != nil {
			return fmt.Errorf("insert exchange %s: %w", ex.ID, err)
		}
	}

	return tx.Commit()
}

// LoadAllExchanges reads all captured exchanges from the database, ordered by timestamp.
func LoadAllExchanges(db *sql.DB) ([]capture.CapturedExchange, error) {
	rows, err := db.Query(`
		SELECT id, timestamp, method, path, query_string,
			request_headers, request_body_schema, request_content_type,
			response_status, response_headers, response_body_schema, response_content_type,
			latency_ms
		FROM captured_exchanges
		ORDER BY timestamp ASC`)
	if err != nil {
		return nil, fmt.Errorf("query exchanges: %w", err)
	}
	defer rows.Close()

	var exchanges []capture.CapturedExchange
	for rows.Next() {
		var ex capture.CapturedExchange
		var ts string
		var reqHeaders, respHeaders string
		var reqBodySchema, respBodySchema sql.NullString

		err := rows.Scan(
			&ex.ID, &ts, &ex.Method, &ex.Path, &ex.QueryString,
			&reqHeaders, &reqBodySchema, &ex.RequestContentType,
			&ex.ResponseStatus, &respHeaders, &respBodySchema, &ex.ResponseContentType,
			&ex.LatencyMs,
		)
		if err != nil {
			return nil, fmt.Errorf("scan exchange: %w", err)
		}

		ex.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		json.Unmarshal([]byte(reqHeaders), &ex.RequestHeaders)
		json.Unmarshal([]byte(respHeaders), &ex.ResponseHeaders)
		if reqBodySchema.Valid {
			ex.RequestBodySchema = json.RawMessage(reqBodySchema.String)
		}
		if respBodySchema.Valid {
			ex.ResponseBodySchema = json.RawMessage(respBodySchema.String)
		}

		exchanges = append(exchanges, ex)
	}

	return exchanges, rows.Err()
}

// CountExchanges returns the total number of captured exchanges.
func CountExchanges(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM captured_exchanges").Scan(&count)
	return count, err
}

func nullableJSON(data json.RawMessage) any {
	if len(data) == 0 {
		return nil
	}
	return string(data)
}
