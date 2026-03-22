# REST Lens Capture

API traffic capture sidecar for [REST Lens](https://restlens.com). Runs as a reverse proxy, captures request/response metadata, and exports inferred OpenAPI specs — all without storing raw data.

## Quick Start

```bash
# Run as a proxy in front of your API
restlens-capture proxy --target http://localhost:3000 --port 9000

# View captured traffic
restlens-capture report

# Export as OpenAPI 3.1
restlens-capture export --openapi
```

## How It Works

```
                ┌───────────────────────┐
  Client ──────▶│  restlens-capture     │──────▶ Your API
  (port 9000)  │  (reverse proxy)      │  (localhost:3000)
                │                       │
                │  ● Captures metadata  │
                │  ● Infers schemas     │
                │  ● Stores in SQLite   │
                └───────────────────────┘
```

REST Lens Capture sits in front of your API as a transparent reverse proxy. For every request/response pair, it:

1. **Forwards traffic** — zero modification, your API behaves identically
2. **Infers JSON schemas** from request/response bodies (types and structure only, never raw values)
3. **Stores metadata** locally in SQLite (method, path, status, headers, inferred schemas, latency)

## Privacy First

REST Lens Capture **never stores raw request/response values**. It infers structural JSON Schema from bodies:

```json
// What your API returns:
{"users": [{"id": 1, "name": "Alice", "email": "alice@example.com"}]}

// What REST Lens Capture stores:
{"type": "object", "properties": {"users": {"type": "array", "items": {"type": "object", "properties": {"id": {"type": "integer"}, "name": {"type": "string"}, "email": {"type": "string"}}}}}}
```

No PII, no secrets, no raw data — just types and shapes.

## Installation

### Binary

Download from [Releases](https://github.com/Edthing/restlens-capture/releases).

### Docker

```bash
docker pull ghcr.io/edthing/restlens-capture:latest
```

### From Source

```bash
go install github.com/Edthing/restlens-capture@latest
```

## Commands

### `proxy` — Capture traffic

```bash
restlens-capture proxy --target http://localhost:3000 --port 9000
```

| Flag | Default | Description |
|------|---------|-------------|
| `--target` | (required) | URL to proxy traffic to |
| `--port` | `9000` | Port to listen on |
| `--capture-headers` | `true` | Capture request/response headers |
| `--capture-bodies` | `true` | Capture body schemas |
| `--db` | `restlens-capture.db` | SQLite database path |

### `report` — View traffic summary

```bash
restlens-capture report
restlens-capture report --format json
```

```
restlens-capture Report
=======================

Captured 73 requests across 7 endpoints
Period: 2026-03-22 13:46 to 2026-03-22 13:53

ENDPOINT                            HITS  STATUS CODES  AVG (ms)  P95 (ms)
--------                            ----  ------------  --------  --------
GET /api/users/{id}                 32    200(28) 404(4) 125      246
POST /api/users                     18    201(18)        78       230
GET /api/projects/{id}/specs        13    200(13)        106      161
PUT /api/projects/{id}/rules/{id}   10    200(10)        64       131
```

### `export` — Generate OpenAPI spec

```bash
# To stdout
restlens-capture export --openapi

# To file
restlens-capture export --openapi -o api.yaml
```

Generates an OpenAPI 3.1 spec with:
- Paths inferred from traffic patterns (`/users/123`, `/users/456` → `/users/{id}`)
- Request/response schemas from captured body shapes
- Observed status codes per endpoint

## Kubernetes Sidecar

REST Lens Capture is designed to run as a k8s sidecar. Add it to your pod and route your Service through it:

```yaml
spec:
  containers:
    - name: app
      image: my-app:latest
      ports:
        - containerPort: 3000

    - name: restlens-capture
      image: ghcr.io/edthing/restlens-capture:latest
      args: ["proxy", "--target=http://localhost:3000", "--port=9000"]
      ports:
        - containerPort: 9000
      livenessProbe:
        httpGet:
          path: /healthz
          port: 9000
      resources:
        requests:
          cpu: 25m
          memory: 32Mi
---
apiVersion: v1
kind: Service
metadata:
  name: my-app
spec:
  ports:
    - port: 80
      targetPort: 9000  # Route through the sidecar
```

See [`k8s/sidecar.yaml`](k8s/sidecar.yaml) for a complete example.

## Production Safety

REST Lens Capture is designed to never impact your application:

- **Circuit breaker** — if capture fails repeatedly (50 failures), the proxy degrades to a transparent passthrough. Resets after 30s.
- **Panic recovery** — schema inference panics are caught and recovered. Your traffic always flows.
- **Non-blocking writes** — captured data is sent to a buffered channel. If the channel is full, exchanges are dropped silently rather than blocking requests.
- **Smart filtering** — only endpoints that have returned at least one 2xx response are tracked. Scanner spam hitting nonexistent paths is ignored.
- **Body limits** — bodies over 10MB skip schema inference but are still proxied.

## Capture Policy

Not all traffic is stored:

| Response | Endpoint seen 2xx before? | Captured? |
|----------|---------------------------|-----------|
| 2xx | — | Yes (and marks endpoint as known) |
| 4xx/5xx | Yes | Yes |
| 4xx/5xx | No | No (likely scanner/spam) |

This means `/api/users/999` returning 404 is captured (because `/api/users/{id}` has returned 200 before), but `/api/totally-fake` returning 404 is ignored.

## Schema Inference

The schema inferrer handles:

| JSON | Inferred Schema |
|------|----------------|
| `"hello"` | `{"type": "string"}` |
| `42` | `{"type": "integer"}` |
| `3.14` | `{"type": "number"}` |
| `true` | `{"type": "boolean"}` |
| `null` | `{"type": "null"}` |
| `[1, 2, 3]` | `{"type": "array", "items": {"type": "integer"}}` |
| `{"key": "val"}` | `{"type": "object", "properties": {"key": {"type": "string"}}}` |

**Dynamic map keys** (hashes, UUIDs, numeric IDs) are detected and emitted as `additionalProperties` instead of individual properties:

```json
// Input: {"abc123def456": {"count": 3}, "789012fed345": {"count": 5}}
// Schema: {"type": "object", "additionalProperties": {"type": "object", "properties": {"count": {"type": "integer"}}}}
```

## License

[MIT](LICENSE)
