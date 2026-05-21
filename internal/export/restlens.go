package export

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultRestLensURL is the production REST Lens base URL.
const DefaultRestLensURL = "https://restlens.com"

// UploadOptions configures an upload of a generated API profile to REST Lens.
type UploadOptions struct {
	// BaseURL is the REST Lens server (defaults to DefaultRestLensURL when empty).
	BaseURL string
	// Token is a REST Lens project API token (sent as a Bearer token).
	Token string
	// OrgSlug and ProjectSlug identify the target project.
	OrgSlug     string
	ProjectSlug string
	// Tag is an optional version tag for the uploaded specification.
	Tag string
}

// uploadRequest is the body for POST /api/projects/{org}/{slug}/specifications.
type uploadRequest struct {
	Spec json.RawMessage `json:"spec"`
	Tag  string          `json:"tag,omitempty"`
}

// UploadResult is the relevant subset of the specification upload response.
type UploadResult struct {
	Specification struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	} `json:"specification"`
	Evaluation struct {
		Status string `json:"status"`
	} `json:"evaluation"`
}

// UploadToRestLens uploads a generated OpenAPI spec (the inferred API profile)
// to REST Lens via the supported specifications endpoint.
func UploadToRestLens(ctx context.Context, spec *OpenAPISpec, opts UploadOptions) (*UploadResult, error) {
	if spec == nil {
		return nil, fmt.Errorf("no spec to upload")
	}
	if opts.Token == "" {
		return nil, fmt.Errorf("a REST Lens API token is required (use --token or RESTLENS_TOKEN)")
	}
	if opts.OrgSlug == "" || opts.ProjectSlug == "" {
		return nil, fmt.Errorf("organization and project slugs are required (use --project <org>/<project>)")
	}

	base := strings.TrimRight(opts.BaseURL, "/")
	if base == "" {
		base = DefaultRestLensURL
	}

	specJSON, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to encode spec: %w", err)
	}

	payload, err := json.Marshal(uploadRequest{Spec: specJSON, Tag: opts.Tag})
	if err != nil {
		return nil, fmt.Errorf("failed to encode request body: %w", err)
	}

	endpoint := fmt.Sprintf("%s/api/projects/%s/%s/specifications",
		base, url.PathEscape(opts.OrgSlug), url.PathEscape(opts.ProjectSlug))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opts.Token)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("REST Lens upload failed (HTTP %d): %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result UploadResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse upload response: %w", err)
	}
	return &result, nil
}
