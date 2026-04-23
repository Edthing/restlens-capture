FROM golang:1.24 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /restlens-capture .

# Prep an empty /data directory so the final image exports it pre-owned by
# nonroot. Without this, named docker volumes and bind mounts inherit the
# root-owned default and the nonroot user can't write capture.db.
RUN mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /restlens-capture /restlens-capture
COPY --from=builder --chown=nonroot:nonroot /out/data /data

WORKDIR /data
USER nonroot:nonroot

ENTRYPOINT ["/restlens-capture"]
