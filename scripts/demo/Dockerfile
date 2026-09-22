# Windshift Demo - Multi-stage Docker Build
# Creates a self-contained demo instance with pre-seeded data and Caddy reverse proxy
#
# This Dockerfile expects the build context to be the PROJECT ROOT, not the demo directory.
#
# Usage:
#   From project root:  docker build -f scripts/demo/Dockerfile -t windshift-demo .
#   With compose:       docker compose -f scripts/demo/docker-compose.yml up --build

# ============================================================
# Stage 1a: Build frontend (pinned node image; the frontend enforces
# engine-strict on an exact node version, so apk's drifting node is not usable)
# ============================================================
FROM node:24.18.0-alpine@sha256:a0b9bf06e4e6193cf7a0f58816cc935ff8c2a908f81e6f1a95432d679c54fbfd AS frontend-builder

WORKDIR /build

# Copy package files first for better layer caching
COPY frontend/package*.json frontend/.npmrc ./
RUN npm ci

# Copy frontend source and build
COPY frontend/ ./
RUN npm run build

# ============================================================
# Stage 1b: Build windshift server binary
# ============================================================
FROM golang:1.27.0-alpine@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS builder

# Install build dependencies
RUN apk add --no-cache git

WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Copy pre-built frontend from the frontend-builder stage
COPY --from=frontend-builder /build/dist ./frontend/dist

# Build backend with static linking
RUN CGO_ENABLED=0 go build -ldflags '-s -w' -o windshift main.go

# ============================================================
# Stage 2: Generate demo data (seeder)
# ============================================================
FROM node:24.18.0-alpine@sha256:a0b9bf06e4e6193cf7a0f58816cc935ff8c2a908f81e6f1a95432d679c54fbfd AS seeder

RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy the windshift binary from builder
COPY --from=builder /build/windshift /usr/local/bin/

# Copy demo generation scripts from builder
COPY --from=builder /build/scripts/demo/generate-demo.js ./
COPY --from=builder /build/scripts/demo/demo-data.js ./
COPY --from=builder /build/scripts/demo/challenge-data.js ./
COPY --from=builder /build/scripts/demo/scale-data.js ./

# Create data directory
RUN mkdir -p /data

# Generate demo database:
# 1. Start windshift server in background (session signing requires a secret;
#    a fixed value is fine here because seeded sessions do not outlive this stage)
# 2. Wait for it to be ready
# 3. Run demo generator
# 4. Stop server
RUN SESSION_SECRET=demo-seeder-not-a-real-credential /usr/local/bin/windshift -db /data/demo.db -p 8080 --allowed-hosts localhost & \
    SERVER_PID=$! && \
    echo "Waiting for server to start..." && \
    sleep 5 && \
    until wget -q --spider http://localhost:8080/api/setup/status 2>/dev/null; do \
        echo "Waiting for server..." && sleep 1; \
    done && \
    echo "Server ready, generating demo data..." && \
    node generate-demo.js --no-server --base-url http://localhost:8080 && \
    echo "Demo data generated, stopping server..." && \
    kill $SERVER_PID 2>/dev/null || true && \
    wait $SERVER_PID 2>/dev/null || true && \
    echo "Seeding complete!"

# ============================================================
# Stage 3: Runtime with Caddy reverse proxy
# ============================================================
FROM caddy:2-alpine

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata gettext su-exec

# Create non-root user for windshift
RUN addgroup -g 1000 windshift && \
    adduser -D -u 1000 -G windshift windshift

# Create data directories
RUN mkdir -p /data /data/attachments && \
    chown -R windshift:windshift /data

# Copy windshift binary from builder
COPY --from=builder /build/windshift /usr/local/bin/
RUN chmod +x /usr/local/bin/windshift

# Copy pre-seeded demo database from seeder
COPY --from=seeder /data/demo.db /data/demo.db
RUN chown windshift:windshift /data/demo.db

# Copy Caddy configuration template and entrypoint from builder
COPY --from=builder /build/scripts/demo/Caddyfile.template /etc/caddy/Caddyfile.template
COPY --from=builder /build/scripts/demo/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Expose ports
# 80  - HTTP (required for Let's Encrypt ACME challenge)
# 443 - HTTPS (main access)
# 8443 - Alternative HTTPS port
EXPOSE 80 443 8443

# Environment variables
ENV HOSTNAME=localhost
ENV WINDSHIFT_PORT=8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:${WINDSHIFT_PORT}/api/setup/status || exit 1

# Note: Container starts as root for Caddy to bind privileged ports (80/443).
# Caddy handles privilege dropping internally, and windshift runs as the
# windshift user via su-exec in the entrypoint.
# See: https://caddyserver.com/docs/conventions#security

ENTRYPOINT ["/entrypoint.sh"]
