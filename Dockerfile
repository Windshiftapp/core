# Multi-stage build for Windshift server

# Stage 1: Build frontend
FROM node:22-alpine AS frontend-builder

WORKDIR /build

# Copy package files first for better layer caching
COPY frontend/package*.json ./

# Install dependencies (npm ci is faster and more reliable for CI)
RUN npm ci

# Copy frontend source and build
COPY frontend/ ./
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.24.6-alpine AS builder

# Install build dependencies (no gcc/musl-dev needed - pure Go SQLite driver)
RUN apk add --no-cache git tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Copy source code
COPY . .

# Copy pre-built frontend from frontend-builder stage
# Static files (JS/CSS/HTML) are architecture-independent
COPY --from=frontend-builder /build/dist ./frontend/dist

# Build backend (pure Go, no CGO needed)
RUN CGO_ENABLED=0 \
    go build -ldflags '-s -w' \
    -o windshift main.go

# Create data directory with placeholder file for proper volume initialization
# Docker only copies ownership to named volumes when there are actual files present
# Empty directories alone don't trigger the volume initialization with correct permissions
RUN mkdir -p /data/attachments && \
    touch /data/.keep /data/attachments/.keep && \
    chown -R 65534:65534 /data

# Stage 3: Scratch runtime (minimal image)
FROM scratch

# Copy CA certificates for HTTPS requests
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy binary
COPY --from=builder /build/windshift /windshift

# Copy data directory with correct ownership (65534:65534)
# This ensures named volumes inherit proper permissions on first mount
COPY --from=builder --chown=65534:65534 /data /data

# Expose default port
EXPOSE 8080

# Default environment variables (parsed directly by Go binary)
ENV PORT=8080
ENV DB_PATH=/data/windshift.db
ENV ATTACHMENT_PATH=/data/attachments

USER 65534:65534

# Run the binary directly (no shell needed)
ENTRYPOINT ["/windshift"]
