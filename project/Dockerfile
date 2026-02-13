# syntax=docker/dockerfile:1

# ---- Build Stage ----
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Cache dependencies (BuildKit cache mounts for Go module + build cache)
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Build the binary with all production optimisations
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
      -ldflags="-s -w" \
      -trimpath \
      -o /app/server .

# ---- Runtime Stage ----
FROM gcr.io/distroless/static-debian12

COPY --from=builder /app/server /server

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/server"]
