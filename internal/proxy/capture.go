package proxy

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Edthing/restlens-capture/internal/capture"
	"github.com/Edthing/restlens-capture/internal/export"
	"github.com/Edthing/restlens-capture/internal/storage"
	"github.com/google/uuid"
)

// CaptureTransport wraps an http.RoundTripper to capture request/response metadata.
// If capture fails or the circuit breaker trips, traffic still flows — capture is
// best-effort and never blocks the proxy path.
//
// Capture policy:
//   - 2xx responses are always captured, and their path pattern is marked as "known"
//   - Non-2xx responses are captured only if the path pattern has seen a 2xx before
//   - This filters out spam/scanners hitting nonexistent endpoints while still
//     capturing legitimate errors (404 on a real resource, 500 on a known endpoint)
type CaptureTransport struct {
	Transport      http.RoundTripper
	captureCh      chan *capture.CapturedExchange
	captureHeaders bool
	captureBodies  bool
	circuit        *CircuitBreaker
	knownPatterns  map[string]struct{}
	patternsMu     sync.RWMutex
}

func NewCaptureTransport(db *sql.DB, captureHeaders, captureBodies bool) *CaptureTransport {
	ch := make(chan *capture.CapturedExchange, 1000)
	ct := &CaptureTransport{
		Transport:      http.DefaultTransport,
		captureCh:      ch,
		captureHeaders: captureHeaders,
		captureBodies:  captureBodies,
		circuit:        NewCircuitBreaker(50, 30*time.Second),
		knownPatterns:  make(map[string]struct{}),
	}

	go ct.writer(db)
	return ct
}

func (ct *CaptureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// If circuit is open, just forward — don't touch the request at all
	if !ct.circuit.AllowCapture() {
		return ct.Transport.RoundTrip(req)
	}

	resp, err := ct.captureRoundTrip(req)
	if err != nil {
		return resp, err
	}
	return resp, nil
}

// captureRoundTrip does the actual capture work, wrapped in a defer/recover
// so panics in schema inference never take down the proxy.
func (ct *CaptureTransport) captureRoundTrip(req *http.Request) (resp *http.Response, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("capture panic (recovered, circuit recording failure): %v", r)
			ct.circuit.RecordFailure()
			// If we already have a response, return it. Otherwise the caller gets the error.
			if resp == nil {
				resp, err = ct.Transport.RoundTrip(req)
			}
		}
	}()

	start := time.Now()

	ex := &capture.CapturedExchange{
		ID:        uuid.New().String(),
		Timestamp: start.UTC(),
		Method:    req.Method,
		Path:      req.URL.Path,
	}

	if req.URL.RawQuery != "" {
		ex.QueryString = req.URL.RawQuery
	}

	if ct.captureHeaders {
		ex.RequestHeaders = flattenHeaders(req.Header)
		ex.RequestContentType = req.Header.Get("Content-Type")
	}

	// Buffer request body for schema inference
	if ct.captureBodies && req.Body != nil {
		bodyBytes, truncated, readErr := readBody(req.Body)
		if readErr == nil {
			req.Body = bodyReader(bodyBytes)
			req.ContentLength = int64(len(bodyBytes))
			if !truncated && isJSON(ex.RequestContentType) {
				ex.RequestBodySchema = capture.InferSchemaFromBytes(bodyBytes)
			}
		}
	}

	resp, err = ct.Transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	ex.LatencyMs = time.Since(start).Milliseconds()
	ex.ResponseStatus = resp.StatusCode

	if ct.captureHeaders {
		ex.ResponseHeaders = flattenHeaders(resp.Header)
		ex.ResponseContentType = resp.Header.Get("Content-Type")
	}

	// Buffer response body for schema inference
	if ct.captureBodies && resp.Body != nil {
		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
		resp.Body.Close()
		if readErr == nil {
			resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			resp.ContentLength = int64(len(bodyBytes))
			if len(bodyBytes) <= maxBodySize && isJSON(ex.ResponseContentType) {
				ex.ResponseBodySchema = capture.InferSchemaFromBytes(bodyBytes)
			}
		}
	}

	pattern := export.NormalizeSinglePath(ex.Path)
	is2xx := ex.ResponseStatus >= 200 && ex.ResponseStatus < 300

	if is2xx {
		// 2xx: always capture and mark pattern as known
		ct.patternsMu.Lock()
		ct.knownPatterns[pattern] = struct{}{}
		ct.patternsMu.Unlock()

		ct.sendCapture(ex)
	} else {
		// Non-2xx: only capture if this pattern has returned 2xx before
		ct.patternsMu.RLock()
		_, known := ct.knownPatterns[pattern]
		ct.patternsMu.RUnlock()

		if known {
			ct.sendCapture(ex)
		}
	}

	return resp, nil
}

func (ct *CaptureTransport) sendCapture(ex *capture.CapturedExchange) {
	select {
	case ct.captureCh <- ex:
		ct.circuit.RecordSuccess()
	default:
		ct.circuit.RecordFailure()
	}
}

// Close signals the writer to stop and waits for pending writes.
func (ct *CaptureTransport) Close() {
	close(ct.captureCh)
}

func (ct *CaptureTransport) writer(db *sql.DB) {
	batch := make([]*capture.CapturedExchange, 0, 100)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := storage.InsertExchangeBatch(db, batch); err != nil {
			log.Printf("failed to write batch: %v", err)
		}
		batch = batch[:0]
	}

	for {
		select {
		case ex, ok := <-ct.captureCh:
			if !ok {
				flush()
				return
			}
			batch = append(batch, ex)
			if len(batch) >= 100 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func flattenHeaders(h http.Header) map[string]string {
	result := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) == 1 {
			result[k] = v[0]
		} else {
			b, _ := json.Marshal(v)
			result[k] = string(b)
		}
	}
	return result
}
