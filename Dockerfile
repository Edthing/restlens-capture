FROM golang:1.24 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /restlens-capture .

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /restlens-capture /restlens-capture

USER nonroot:nonroot

ENTRYPOINT ["/restlens-capture"]
