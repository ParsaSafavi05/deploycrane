# syntax=docker/dockerfile:1.7

# ─── Build stage ─────────────────────────────────────────────────────────────
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

# Only what's actually needed to build — drop unused `make`
RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Download deps as a separate layer — cache busts only when go.mod/go.sum change
COPY go.mod go.sum ./
# Cache mounts keep the module cache across builds (requires BuildKit)
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download -x

COPY . .

# BuildKit injects these from --platform flags
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=0.1.0-dev

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -trimpath \
    -ldflags="-w -s -X main.Version=${VERSION}" \
    -o deploycrane ./cmd/deploycrane

# ─── Runtime stage ───────────────────────────────────────────────────────────
# Pin to a specific Alpine version — never use `latest`
FROM alpine:3.22

RUN apk add --no-cache ca-certificates tini

# Create user AND the runtime directories in one layer,
# before switching ownership — fixes the missing /app/data & /app/clones bug
RUN addgroup -g 1000 app && \
    adduser -u 1000 -G app -S app && \
    mkdir -p /app/data /app/clones

WORKDIR /app

COPY --from=builder /build/deploycrane /app/deploycrane

RUN chown -R app:app /app

# Declare persistent directories so orchestrators treat them as volumes
VOLUME ["/app/data", "/app/clones"]

USER app

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
    # Don't default to wildcard — operators should set this explicitly
    CORS_ORIGINS=""

EXPOSE 8080

# More realistic start-period for DB init; wget is busybox-included in Alpine
HEALTHCHECK --interval=30s --timeout=10s --start-period=20s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:${LISTEN_PORT}/health || exit 1

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["/app/deploycrane"]