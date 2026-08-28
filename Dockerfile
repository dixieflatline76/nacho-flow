# syntax=docker/dockerfile:1
# Multi-stage ultra-lightweight distroless Docker image for Nacho Flow
# Total image footprint: < 15MB

# Stage 1: Build static binary
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source tree
COPY . .

# Build statically linked stripped binary with version metadata
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w -X 'main.version=${VERSION}'" \
    -trimpath \
    -o /bin/nacho-flow \
    ./cmd/nacho-flow

# Stage 2: Distroless minimal runtime
FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="Nacho Flow" \
      org.opencontainers.image.description="High-performance OpenAI-compatible hybrid AI gateway for local GPUs and cloud APIs" \
      org.opencontainers.image.url="https://spicebox.dev/nacho-flow/" \
      org.opencontainers.image.source="https://github.com/dixieflatline76/nacho-flow" \
      org.opencontainers.image.licenses="AGPL-3.0-or-later"

WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /bin/nacho-flow /usr/local/bin/nacho-flow

# Default configuration volume mount
VOLUME ["/config"]

# Expose default HTTP gateway port
EXPOSE 8000

# Run as unprivileged nonroot user (UID 65532)
USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/nacho-flow", "-config", "/config/config.yaml"]
