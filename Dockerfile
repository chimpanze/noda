ARG VARIANT=slim

# Editor build stage
FROM node:22-bookworm-slim AS editor

WORKDIR /editor

COPY editor/package.json editor/package-lock.json* ./
RUN npm ci

COPY editor/ .
COPY docs/ /docs
RUN npm run build

# Go builder stage
FROM golang:1.25-bookworm AS builder

ARG VARIANT

WORKDIR /build

# Install libvips only for the full variant
RUN if [ "$VARIANT" = "full" ]; then \
    apt-get update && apt-get install -y --no-install-recommends \
        libvips-dev \
        pkg-config \
    && rm -rf /var/lib/apt/lists/*; \
    fi

# Cache Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source and embed built editor assets
COPY . .
COPY --from=editor /editor/dist editorfs/dist

RUN if [ "$VARIANT" = "full" ]; then \
        CGO_ENABLED=1 go build -tags embed_editor -ldflags "\
            -X main.Version=$(git describe --tags --always --dirty 2>/dev/null || echo 0.0.1-dev) \
            -X main.Commit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown) \
            -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
            -o /noda ./cmd/noda; \
    else \
        CGO_ENABLED=0 go build -tags noimage,embed_editor -ldflags "\
            -X main.Version=$(git describe --tags --always --dirty 2>/dev/null || echo 0.0.1-dev) \
            -X main.Commit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown) \
            -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
            -o /noda ./cmd/noda; \
    fi

# Runtime stage: full variant
FROM debian:bookworm-slim AS runtime-full

RUN apt-get update && apt-get install -y --no-install-recommends \
    libvips \
    ca-certificates \
    tzdata \
    wget \
    && rm -rf /var/lib/apt/lists/*

RUN groupadd -r noda && useradd -r -g noda -d /home/noda -m noda

COPY --from=builder /noda /noda

USER noda

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:3000/health/live || exit 1

ENTRYPOINT ["/noda"]

# Runtime stage: slim variant (distroless)
FROM gcr.io/distroless/static-debian12 AS runtime-slim

COPY --from=builder /noda /noda

USER nonroot

ENTRYPOINT ["/noda"]

# Final stage: select variant
FROM runtime-${VARIANT}
