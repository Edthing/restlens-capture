package proxy

import (
	"bytes"
	"io"
	"strings"
)

const maxBodySize = 10 * 1024 * 1024 // 10MB

// readBody reads up to maxBodySize bytes from the reader.
// Returns the bytes read and whether the body was truncated.
func readBody(r io.ReadCloser) ([]byte, bool, error) {
	if r == nil {
		return nil, false, nil
	}
	defer r.Close()

	lr := io.LimitReader(r, maxBodySize+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, false, err
	}

	truncated := len(data) > maxBodySize
	if truncated {
		data = data[:maxBodySize]
	}
	return data, truncated, nil
}

// isJSON checks if the content type indicates JSON.
func isJSON(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "application/json") ||
		strings.Contains(ct, "+json")
}

// bodyReader wraps bytes into a ReadCloser.
func bodyReader(data []byte) io.ReadCloser {
	return io.NopCloser(bytes.NewReader(data))
}
