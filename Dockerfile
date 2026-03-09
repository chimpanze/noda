# Editor build stage
FROM node:22-slim AS editor
WORKDIR /editor
COPY editor/package.json editor/package-lock.json ./
RUN npm ci --silent
COPY editor/ .
RUN npm run build

# Go builder stage
FROM golang:1.25-bookworm AS builder

WORKDIR /build

# Install libvips for bimg CGO compilation
RUN apt-get update && apt-get install -y --no-install-recommends \
    libvips-dev \
    pkg-config \
    && rm -rf /var/lib/apt/lists/*

# Cache Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source and embed editor assets
COPY . .
COPY --from=editor /editor/dist editorfs/dist
RUN CGO_ENABLED=1 go build -tags embed_editor -o /noda ./cmd/noda

# Runtime stage
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libvips \
    ca-certificates \
    tzdata \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /noda /noda

ENTRYPOINT ["/noda"]
