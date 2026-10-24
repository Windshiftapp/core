# syntax=docker/dockerfile:1
# =============================================================================
# windshift-runner image
#
# Runs the remote runner as a container instead of via the host install script.
# It talks to the HOST Docker daemon over the mounted socket and asks it to
# spawn sibling agent containers (Docker-out-of-Docker) — the agent never runs
# nested inside this container.
#
# Built from the repo root (needs go.mod + windshift-runner/ + cmd/windshift-triage):
#   docker build -f deploy/windshift-runner/Dockerfile -t windshift/runner:local .
#
# Run (see deploy/windshift-runner/runner-compose.yml for the full example):
#   docker run -d \
#     -e WS_API_URL=... -e WSRUNNER_REGISTRATION_TOKEN=wsrt_... \
#     -e WSRUNNER_IMAGE=ghcr.io/windshiftapp/windshift-agent:latest \
#     -v /var/run/docker.sock:/var/run/docker.sock \
#     -v /var/lib/windshift-runner:/var/lib/windshift-runner \
#     ghcr.io/windshiftapp/windshift-runner:latest
#
# The /var/lib/windshift-runner same-path mount matters: the host daemon (not
# this container) bind-mounts each prepared checkout into the agent container,
# so the checkout must exist on the host at the path this container sees.
# =============================================================================

# --------------------------------------------------------------------
# Stage 1: build windshift-runner + windshift-triage
# --------------------------------------------------------------------
FROM golang:1.27.3-alpine@sha256:f2c872bcee949b7be9c490efd02b48a8a2fbf947cdf533f1c58525ec9d8170fd AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Populated by BuildKit when --platform is passed (CI does); empty otherwise,
# which makes Go build for the native arch. A hard =amd64 default here once
# shipped x86_64 binaries inside the arm64 image — they "worked" only via
# qemu-user binfmt emulation on the runner host (WI-394).
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/windshift-runner ./windshift-runner
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/windshift-triage ./cmd/windshift-triage

# --------------------------------------------------------------------
# Stage 2: runtime — runner + triage + git + docker CLI
# --------------------------------------------------------------------
FROM alpine:3.21@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d

# git: triage shells out to it. docker-cli: drive the host daemon via the
# mounted socket. ca-certificates: TLS to the orchestrator.
RUN apk add --no-cache ca-certificates git docker-cli

COPY --from=build /out/windshift-runner  /usr/local/bin/windshift-runner
COPY --from=build /out/windshift-triage /usr/local/bin/windshift-triage

# Defaults; the cache root is expected to be a host bind-mount (same path).
ENV WSRUNNER_TRIAGE_BIN=/usr/local/bin/windshift-triage \
    WSRUNNER_CACHE_ROOT=/var/lib/windshift-runner/cache
RUN mkdir -p /var/lib/windshift-runner/cache

# Runs as root so it can reach the mounted Docker socket; the agent containers
# it spawns are the sandboxed, non-root ones.
ENTRYPOINT ["/usr/local/bin/windshift-runner"]
