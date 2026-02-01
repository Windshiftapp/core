# Multi-stage build for Windshift server

# Stage 1: Build frontend on HOST platform (no QEMU emulation needed)
# Using --platform=$BUILDPLATFORM ensures this runs natively on x86-64
# which avoids QEMU issues with native Node.js binaries (esbuild, rollup, etc.)
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend-builder

WORKDIR /build

# Copy package files first for better layer caching
COPY frontend/package*.json ./

# Install dependencies (npm ci is faster and more reliable for CI)
RUN npm ci

# Copy frontend source and build
COPY frontend/ ./
RUN npm run build

# Stage 2: Build Go binary (target platform, uses QEMU for arm64)
FROM golang:1.24.6-alpine AS builder

# Install build dependencies (nodejs/npm no longer needed)
RUN apk add --no-cache gcc musl-dev git tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Copy source code
COPY . .

# Copy pre-built frontend from frontend-builder stage
# Static files (JS/CSS/HTML) are architecture-independent
COPY --from=frontend-builder /build/dist ./frontend/dist

# Build backend with static linking
RUN CGO_ENABLED=1 \
    go build -ldflags '-s -w -linkmode external -extldflags "-static"' \
    -o windshift main.go

# Stage 3: Scratch runtime (minimal image)
FROM scratch

# Copy CA certificates for HTTPS requests
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy binary
COPY --from=builder /build/windshift /windshift

# Expose default port
EXPOSE 8080

# Default environment variables (parsed directly by Go binary)
ENV PORT=8080
ENV DB_PATH=/data/windshift.db
ENV ATTACHMENT_PATH=/data/attachments

USER 65534:65534

# Run the binary directly (no shell needed)
ENTRYPOINT ["/windshift"]
