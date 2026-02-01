# Multi-stage build for Windshift server
# Stage 1: Build the application
FROM golang:1.24.6-alpine AS builder

# Install build dependencies
RUN apk add --no-cache gcc musl-dev nodejs npm git tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Copy source code
COPY . .

# Build frontend - clean install with explicit platform targeting
WORKDIR /build/frontend
ARG TARGETPLATFORM
ARG TARGETARCH
RUN rm -rf node_modules package-lock.json && \
    npm config set target_arch ${TARGETARCH:-x64} && \
    npm config set target_platform linux && \
    npm install --force --ignore-scripts && \
    npm rebuild && \
    npm run build

# Build backend with static linking (native architecture)
WORKDIR /build
RUN CGO_ENABLED=1 \
    go build -ldflags '-s -w -linkmode external -extldflags "-static"' \
    -o windshift main.go

# Stage 2: Scratch runtime (minimal image)
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
