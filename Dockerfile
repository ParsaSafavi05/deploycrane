# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make ca-certificates

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -a -installsuffix cgo \
    -ldflags="-w -s" \
    -o deploycrane ./cmd/deploycrane

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tini

# Create non-root user
RUN addgroup -g 1000 app && \
    adduser -u 1000 -G app -S app

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/deploycrane /app/deploycrane

# Change ownership to non-root user
RUN chown -R app:app /app

# Switch to non-root user
USER app

# Default environment variables (can be overridden)
ENV LISTEN_PORT=8080 \
    SERVER_ADDR=0.0.0.0 \
    DB_PATH=/app/data/deploycrane.db \
    CLONE_BASE_DIR=/app/clones \
    IMAGE_PREFIX=deploycrane \
    PORT_ALLOCATION_MIN=8000 \
    PORT_ALLOCATION_MAX=9000 \
    READ_TIMEOUT=15s \
    WRITE_TIMEOUT=15s \
    IDLE_TIMEOUT=60s \
    SHUTDOWN_TIMEOUT=30s \
    CORS_ORIGINS=*

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Use tini to handle signals properly
ENTRYPOINT ["/sbin/tini", "--"]

# Run the application
CMD ["/app/deploycrane"]
