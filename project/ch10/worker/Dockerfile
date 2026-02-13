# syntax=docker/dockerfile:1
# worker/Dockerfile — Production worker build.

FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
      -ldflags="-s -w" \
      -trimpath \
      -o /app/worker ./worker

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app/worker /worker

EXPOSE 8081

USER nonroot:nonroot

ENTRYPOINT ["/worker"]
