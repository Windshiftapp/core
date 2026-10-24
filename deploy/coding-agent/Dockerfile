# =============================================================================
# Windshift ws-carrier image
#
# A thin image whose ONLY purpose is to ship the cross-built `ws` CLI at
# /usr/local/bin/ws. The windshift-agent image (the node-free codehamr fork,
# WI-204 — the actual coding agent) lifts `ws` from here via its WS_IMAGE build
# arg, because `ws` lives in this repo while the agent lives in the sibling
# windshift-agent repo.
#
# This image no longer bakes the retired Node agent, Node/npm, the windshift-guard
# extension, or an RPC entrypoint — those moved out when the coding agent was
# replaced by windshift-agent (WI-238). The hardened `docker run` sandbox is
# still assembled by the windshift-runner (DockerAgentRunner) around whichever
# agent image the operator points WSRUNNER_IMAGE at.
#
# Built from the *repo root* (the build context needs cmd/ws + go.mod):
#
#   docker build -f deploy/coding-agent/Dockerfile -t windshift/ws-carrier:local .
# =============================================================================

# --------------------------------------------------------------------
# Stage 1: cross-compile the ws CLI for linux/<TARGETARCH>
# --------------------------------------------------------------------
FROM golang:1.27.3-bookworm@sha256:32fa283222e2905d9fee971f60c842e5c815fdc036cf7ea599bf36bd7ca8f67f AS ws-build

WORKDIR /src

# Cache module downloads ahead of source copy so changes outside cmd/ws
# don't blow the layer cache.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Populated by BuildKit when --platform is passed (CI does); empty otherwise,
# which makes Go build for the native arch. A hard =amd64 default here once
# shipped an x86_64 ws binary inside the arm64 image (WI-394).
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/ws ./cmd/ws

# --------------------------------------------------------------------
# Stage 2: thin ws-carrier
# --------------------------------------------------------------------
FROM alpine:3.21@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d

RUN apk add --no-cache ca-certificates

COPY --from=ws-build /out/ws /usr/local/bin/ws
RUN chmod +x /usr/local/bin/ws

# Numeric non-root user matching the agent images that consume this CLI.
USER 1000:1000

ENTRYPOINT ["/usr/local/bin/ws"]
