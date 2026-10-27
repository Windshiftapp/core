# Multi-stage build for Windshift server

# Stage 1: Build frontend
FROM node:24.18.0-alpine@sha256:a0b9bf06e4e6193cf7a0f58816cc935ff8c2a908f81e6f1a95432d679c54fbfd AS frontend-builder

WORKDIR /build

# Copy package files first for better layer caching
COPY frontend/package*.json frontend/.npmrc ./

# Install dependencies (npm ci is faster and more reliable for CI).
# The OpenUI packages ship a telemetry postinstall; frontend/.npmrc already sets
# ignore-scripts=true, and these env vars keep it disabled if scripts are ever
# re-enabled.
RUN OPENUI_TELEMETRY_DISABLED=1 DO_NOT_TRACK=1 npm ci

# Build-time version metadata for the static UI. These are passed by
# release.sh / CI so the About page and footer do not fall back to "dev".
ARG VERSION=dev
ARG RELEASE_NAME=""

# Copy frontend source and build
COPY frontend/ ./
RUN VITE_APP_VERSION_CODE="${VERSION}" \
    VITE_APP_VERSION_NAME="${RELEASE_NAME}" \
    npm run build

# Stage 2: Build Go binary
FROM golang:1.27.3-alpine@sha256:f2c872bcee949b7be9c490efd02b48a8a2fbf947cdf533f1c58525ec9d8170fd AS builder

# Install build dependencies (no gcc/musl-dev needed - pure Go SQLite driver)
RUN apk add --no-cache git tzdata

# Build-time version metadata (release.sh forwards these via --build-arg).
# Defaults keep `docker build .` from a developer checkout working.
ARG VERSION=dev
ARG RELEASE_NAME=""
ARG COMMIT=none
ARG BUILD_DATE=unknown

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Copy source code
COPY . .

# Copy pre-built frontend from frontend-builder stage
# Static files (JS/CSS/HTML) are architecture-independent
COPY --from=frontend-builder /build/dist ./frontend/dist

# Build backend (pure Go, no CGO needed). Version metadata is injected via
# ldflags into windshift/internal/version so /api/version reports it.
RUN CGO_ENABLED=0 \
    go build -ldflags "-s -w \
      -X windshift/internal/version.Version=${VERSION#v} \
      -X windshift/internal/version.Commit=${COMMIT} \
      -X windshift/internal/version.Date=${BUILD_DATE} \
      -X 'windshift/internal/version.ReleaseName=${RELEASE_NAME}'" \
    -o windshift main.go

# Create data directory with placeholder file for proper volume initialization
# Docker only copies ownership to named volumes when there are actual files present
# Empty directories alone don't trigger the volume initialization with correct permissions
RUN mkdir -p /data/attachments /data/plugins /data/prompts && \
    touch /data/.keep /data/attachments/.keep /data/plugins/.keep /data/prompts/.keep && \
    chown -R 65534:65534 /data

# Stage 3: Scratch runtime (minimal image)
FROM scratch

# Copy CA certificates for HTTPS requests
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy binary
COPY --from=builder /build/windshift /windshift

# Copy third-party notices required by the Apache-2.0 dependencies (AWS SDK)
COPY --from=builder /build/NOTICE /NOTICE

# Copy data directory with correct ownership (65534:65534)
# This ensures named volumes inherit proper permissions on first mount
COPY --from=builder --chown=65534:65534 /data /data

# Expose default port
EXPOSE 8080

# Default environment variables (parsed directly by Go binary)
ENV PORT=8080
ENV DB_PATH=/data/windshift.db
ENV ATTACHMENT_PATH=/data/attachments
ENV PLUGIN_DIR=/data/plugins
ENV AI_PROMPTS_DIR=/data/prompts

USER 65534:65534

# Run the binary directly (no shell needed)
ENTRYPOINT ["/windshift"]
