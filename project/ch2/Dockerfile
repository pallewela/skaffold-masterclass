# ---- Build Stage ----
# Use the official Go image as the build environment.
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go.mod first for layer caching.
COPY go.mod ./
RUN go mod download

# Copy the source code and build.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/server .

# ---- Runtime Stage ----
FROM gcr.io/distroless/static-debian12

COPY --from=builder /app/server /server

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/server"]
