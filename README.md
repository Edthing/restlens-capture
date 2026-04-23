# REST Lens Capture

API traffic capture sidecar for [REST Lens](https://restlens.com). Runs as a reverse proxy, captures request/response metadata, and exports inferred OpenAPI specs — all without storing raw data.

## Quick Start

```bash
# 1. Start the proxy in front of your API
restlens-capture proxy --target http://localhost:3000 --port 9000

# 2. Point your client at :9000 instead of :3000 and use the app normally

# 3. See what was captured
restlens-capture report

# 4. Export an OpenAPI 3.1 spec
restlens-capture export --openapi -o api.yaml
```

If you'd rather run this as a container, skip to [Run with Docker](#run-with-docker).

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

- **Binary** — download from [Releases](https://github.com/Edthing/restlens-capture/releases)
- **Docker** — `ghcr.io/edthing/restlens-capture:latest` (see [Run with Docker](#run-with-docker))
- **From source** — `go install github.com/Edthing/restlens-capture@latest`

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

## Run with Docker

The published image's entrypoint is the binary — everything after the image name is a normal CLI invocation.

```bash
docker pull ghcr.io/edthing/restlens-capture:latest
```

You need to answer three things for every setup:

1. **How does the proxy reach your backend?** — container name, `host.docker.internal`, or host networking
2. **What port do your clients talk to?** — usually `-p 9000:9000` on the proxy container
3. **Where does the SQLite DB live?** — mount a volume, otherwise captures vanish when the container stops

### Backend running on your host

Most common local setup: backend on host port 3000, you want the proxy on host port 9000, captures persisted under `./capture/`.

```bash
mkdir -p capture
docker run --rm \
  -p 9000:9000 \
  --add-host=host.docker.internal:host-gateway \
  -v "$PWD/capture:/data" \
  ghcr.io/edthing/restlens-capture:latest \
  proxy \
    --target=http://host.docker.internal:3000 \
    --port=9000 \
    --db=/data/capture.db
```

Point your client at `http://localhost:9000` instead of `:3000`.

> **Linux shortcut** — drop the port mapping and use `--network host` instead; the proxy binds directly on host port 9000 and `--target=http://localhost:3000` just works:
> ```bash
> docker run --rm --network host \
>   -v "$PWD/capture:/data" \
>   ghcr.io/edthing/restlens-capture:latest \
>   proxy --target=http://localhost:3000 --port=9000 --db=/data/capture.db
> ```

> **Volume permissions** — the image runs as UID 65532 (distroless nonroot). Named docker volumes (`-v rlc-data:/data`) work out of the box because the image exports `/data` pre-owned by nonroot. For bind mounts to a host directory, either `chown 65532:65532 capture` first, or switch to a named volume.

### Backend in another container

Put both on the same user-defined network and reach the backend by container name:

```bash
docker network create apis
docker run -d --name backend --network apis your-api:latest
docker run -d --name capture \
  --network apis \
  -p 9000:9000 \
  -v rlc-data:/data \
  ghcr.io/edthing/restlens-capture:latest \
  proxy --target=http://backend:3000 --port=9000 --db=/data/capture.db
```

Clients outside docker hit `http://localhost:9000`; the proxy reaches the backend by its DNS name `backend:3000` on the shared network.

### docker-compose

```yaml
services:
  backend:
    image: your-api:latest
    expose: ["3000"]          # not published — proxy reaches it via compose DNS

  capture:
    image: ghcr.io/edthing/restlens-capture:latest
    command:
      - proxy
      - --target=http://backend:3000
      - --port=9000
      - --db=/data/capture.db
    ports: ["9000:9000"]       # clients talk to this one
    volumes:
      - rlc-data:/data
    depends_on: [backend]

volumes:
  rlc-data:
```

### Reading the captured data

`report` and `export` both read the same SQLite DB, so point another container at the same volume:

```bash
# Traffic summary
docker run --rm -v rlc-data:/data \
  ghcr.io/edthing/restlens-capture:latest \
  report --db=/data/capture.db

# OpenAPI 3.1 spec
docker run --rm -v rlc-data:/data \
  ghcr.io/edthing/restlens-capture:latest \
  export --openapi --db=/data/capture.db > api.yaml
```

Or `exec` into the running proxy container directly:

```bash
docker exec capture /restlens-capture report --db=/data/capture.db
```

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
