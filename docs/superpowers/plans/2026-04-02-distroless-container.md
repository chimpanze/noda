# Distroless Container Build Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Docker container as a distroless image for the slim (default) variant, with the full variant retaining debian-slim for libvips support.

**Architecture:** A `noimage` Go build tag conditionally excludes the bimg-based image plugin, enabling `CGO_ENABLED=0` static builds. The Dockerfile uses a `VARIANT` build arg to select between slim (distroless) and full (debian-slim) runtime bases. The CI pipeline builds both variants as multi-arch images with distinct tags.

**Tech Stack:** Go build tags, Docker multi-stage builds, `gcr.io/distroless/static-debian12`, GitHub Actions matrix

**Spec:** `docs/superpowers/specs/2026-04-02-distroless-container-design.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `cmd/noda/main.go` | Modify | Remove `imageplugin` import and `&imageplugin.Plugin{}` from `corePlugins()`, add `optionalPlugins` slice to the return |
| `cmd/noda/plugins_image.go` | Create | `//go:build !noimage` — `init()` appends image plugin to `optionalPlugins` |
| `cmd/noda/plugins_noimage.go` | Create | `//go:build noimage` — empty stub |
| `internal/mcp/plugins.go` | Modify | Remove `imageplugin` import and entry from `corePlugins()`, add `optionalPlugins` slice to the return |
| `internal/mcp/plugins_image.go` | Create | `//go:build !noimage` — `init()` appends image plugin to `optionalPlugins` |
| `internal/mcp/plugins_noimage.go` | Create | `//go:build noimage` — empty stub |
| `Dockerfile` | Modify | Restructure with `VARIANT` build arg, conditional stages for slim/full |
| `.github/workflows/docker.yml` | Modify | Add variant to matrix, split merge into `merge-slim`/`merge-full`, update tagging |

---

### Task 1: Add `noimage` build tag to `cmd/noda`

**Files:**
- Modify: `cmd/noda/main.go:47,730-751`
- Create: `cmd/noda/plugins_image.go`
- Create: `cmd/noda/plugins_noimage.go`

- [ ] **Step 1: Declare `optionalPlugins` slice in `main.go`**

Add a package-level variable in `cmd/noda/main.go` near the top of the file (after the import block, before `var Version`):

```go
// optionalPlugins holds plugins registered via build-tagged init() functions.
// The image plugin is added here when built without the noimage tag.
var optionalPlugins []api.Plugin
```

- [ ] **Step 2: Remove image plugin from `corePlugins()` in `main.go`**

In `cmd/noda/main.go`, modify the `corePlugins()` function:

1. Remove the `imageplugin "github.com/chimpanze/noda/plugins/image"` import (line 47).
2. Remove `&imageplugin.Plugin{},` from the `corePlugins()` return slice (line 742).
3. Append `optionalPlugins` to the returned slice:

```go
func corePlugins() []api.Plugin {
	plugins := []api.Plugin{
		&control.Plugin{},
		&transform.Plugin{},
		&util.Plugin{},
		&workflow.Plugin{},
		&response.Plugin{},
		&dbplugin.Plugin{},
		&cacheplugin.Plugin{},
		&event.Plugin{},
		&corestorage.Plugin{},
		&upload.Plugin{},
		&httpplugin.Plugin{},
		&emailplugin.Plugin{},
		&corews.Plugin{},
		&coresse.Plugin{},
		&corewasm.Plugin{},
		&coreoidc.Plugin{},
		&livekitplugin.Plugin{},
	}
	return append(plugins, optionalPlugins...)
}
```

- [ ] **Step 3: Create `cmd/noda/plugins_image.go`**

```go
//go:build !noimage

package main

import (
	"github.com/chimpanze/noda/pkg/api"
	imageplugin "github.com/chimpanze/noda/plugins/image"
)

func init() {
	optionalPlugins = append(optionalPlugins, &imageplugin.Plugin{})
}
```

- [ ] **Step 4: Create `cmd/noda/plugins_noimage.go`**

```go
//go:build noimage

// Image plugin excluded — built without CGO/libvips support.
package main
```

- [ ] **Step 5: Run tests to verify no regressions**

Run: `go test ./cmd/noda/... -count=1`
Expected: All existing tests pass. The default build (no `noimage` tag) still includes the image plugin.

- [ ] **Step 6: Verify `noimage` build compiles**

Run: `CGO_ENABLED=0 go build -tags noimage -o /dev/null ./cmd/noda`
Expected: Compiles successfully with no errors.

- [ ] **Step 7: Commit**

```bash
git add cmd/noda/main.go cmd/noda/plugins_image.go cmd/noda/plugins_noimage.go
git commit -m "feat: add noimage build tag to exclude image plugin from cmd/noda"
```

---

### Task 2: Add `noimage` build tag to `internal/mcp`

**Files:**
- Modify: `internal/mcp/plugins.go:21,40`
- Create: `internal/mcp/plugins_image.go`
- Create: `internal/mcp/plugins_noimage.go`

- [ ] **Step 1: Declare `optionalPlugins` slice in `plugins.go`**

Add a package-level variable in `internal/mcp/plugins.go` (before the `corePlugins()` function):

```go
// optionalPlugins holds plugins registered via build-tagged init() functions.
var optionalPlugins []api.Plugin
```

- [ ] **Step 2: Remove image plugin from `corePlugins()` in `plugins.go`**

1. Remove the `imageplugin "github.com/chimpanze/noda/plugins/image"` import (line 21).
2. Remove `&imageplugin.Plugin{},` from the `corePlugins()` return slice (line 40).
3. Append `optionalPlugins` to the returned slice:

```go
func corePlugins() []api.Plugin {
	plugins := []api.Plugin{
		&control.Plugin{},
		&transform.Plugin{},
		&util.Plugin{},
		&workflow.Plugin{},
		&response.Plugin{},
		&dbplugin.Plugin{},
		&cacheplugin.Plugin{},
		&event.Plugin{},
		&corestorage.Plugin{},
		&upload.Plugin{},
		&httpplugin.Plugin{},
		&emailplugin.Plugin{},
		&corews.Plugin{},
		&coresse.Plugin{},
		&corewasm.Plugin{},
		&coreoidc.Plugin{},
		&livekitplugin.Plugin{},
	}
	return append(plugins, optionalPlugins...)
}
```

- [ ] **Step 3: Create `internal/mcp/plugins_image.go`**

```go
//go:build !noimage

package mcp

import (
	"github.com/chimpanze/noda/pkg/api"
	imageplugin "github.com/chimpanze/noda/plugins/image"
)

func init() {
	optionalPlugins = append(optionalPlugins, &imageplugin.Plugin{})
}
```

- [ ] **Step 4: Create `internal/mcp/plugins_noimage.go`**

```go
//go:build noimage

// Image plugin excluded — built without CGO/libvips support.
package mcp
```

- [ ] **Step 5: Run tests to verify no regressions**

Run: `go test ./internal/mcp/... -count=1`
Expected: All existing tests pass.

- [ ] **Step 6: Verify `noimage` build still compiles**

Run: `CGO_ENABLED=0 go build -tags noimage -o /dev/null ./cmd/noda`
Expected: Compiles successfully (both `cmd/noda` and `internal/mcp` now support the tag).

- [ ] **Step 7: Commit**

```bash
git add internal/mcp/plugins.go internal/mcp/plugins_image.go internal/mcp/plugins_noimage.go
git commit -m "feat: add noimage build tag to exclude image plugin from internal/mcp"
```

---

### Task 3: Restructure Dockerfile for slim/full variants

**Files:**
- Modify: `Dockerfile`

- [ ] **Step 1: Rewrite the Dockerfile**

Replace the entire `Dockerfile` with:

```dockerfile
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

ARG VARIANT=slim

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
```

- [ ] **Step 2: Verify full variant builds locally**

Run: `docker build --build-arg VARIANT=full -t noda:full-test .`
Expected: Builds successfully, same as current behavior.

- [ ] **Step 3: Verify slim variant builds locally**

Run: `docker build --build-arg VARIANT=slim -t noda:slim-test .`
Expected: Builds successfully with the distroless base.

- [ ] **Step 4: Smoke test both variants**

Run:
```bash
# Full variant
docker run --rm -d --name noda-full-test -p 3001:3000 noda:full-test serve --config /dev/null 2>/dev/null
sleep 3
curl -sf http://localhost:3001/health/live && echo "full: OK"
docker stop noda-full-test

# Slim variant
docker run --rm -d --name noda-slim-test -p 3002:3000 noda:slim-test serve --config /dev/null 2>/dev/null
sleep 3
curl -sf http://localhost:3002/health/live && echo "slim: OK"
docker stop noda-slim-test
```

Expected: Both variants respond on `/health/live`.

- [ ] **Step 5: Verify slim image has no shell**

Run: `docker run --rm --entrypoint sh noda:slim-test -c "echo hi" 2>&1`
Expected: Error (no shell available in distroless).

- [ ] **Step 6: Commit**

```bash
git add Dockerfile
git commit -m "feat: restructure Dockerfile for slim (distroless) and full variants"
```

---

### Task 4: Update CI pipeline for both variants

**Files:**
- Modify: `.github/workflows/docker.yml`

- [ ] **Step 1: Rewrite the workflow**

Replace the entire `.github/workflows/docker.yml` with:

```yaml
name: Docker

on:
  push:
    branches: [main]
    tags: ["v*"]

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}

jobs:
  build:
    strategy:
      matrix:
        include:
          - platform: linux/amd64
            runner: ubuntu-latest
            variant: slim
          - platform: linux/amd64
            runner: ubuntu-latest
            variant: full
          - platform: linux/arm64
            runner: ubuntu-24.04-arm
            variant: slim
          - platform: linux/arm64
            runner: ubuntu-24.04-arm
            variant: full
    runs-on: ${{ matrix.runner }}
    permissions:
      contents: read
      packages: write
    steps:
      - uses: actions/checkout@v6

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v4

      - name: Log in to GHCR
        uses: docker/login-action@v4
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build and push by digest
        id: build
        uses: docker/build-push-action@v7
        with:
          context: .
          platforms: ${{ matrix.platform }}
          build-args: VARIANT=${{ matrix.variant }}
          outputs: type=image,name=${{ env.REGISTRY }}/${{ env.IMAGE_NAME }},push-by-digest=true,name-canonical=true,push=true
          cache-from: type=gha,scope=${{ matrix.platform }}-${{ matrix.variant }}
          cache-to: type=gha,scope=${{ matrix.platform }}-${{ matrix.variant }},mode=max

      - name: Export digest
        run: |
          mkdir -p /tmp/digests
          digest="${{ steps.build.outputs.digest }}"
          touch "/tmp/digests/${digest#sha256:}"

      - name: Upload digest
        uses: actions/upload-artifact@v7
        with:
          name: digests-${{ matrix.runner }}-${{ matrix.variant }}
          path: /tmp/digests/*
          if-no-files-found: error
          retention-days: 1

  merge-slim:
    runs-on: ubuntu-latest
    needs: build
    permissions:
      contents: read
      packages: write
    steps:
      - name: Download digests
        uses: actions/download-artifact@v8
        with:
          path: /tmp/digests
          pattern: digests-*-slim
          merge-multiple: true

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v4

      - name: Log in to GHCR
        uses: docker/login-action@v4
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Extract metadata
        id: meta
        uses: docker/metadata-action@v6
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
          tags: |
            type=ref,event=branch
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=semver,pattern={{major}}

      - name: Create manifest list and push
        working-directory: /tmp/digests
        run: |
          docker buildx imagetools create \
            $(jq -cr '.tags | map("-t " + .) | join(" ")' <<< "$DOCKER_METADATA_OUTPUT_JSON") \
            $(printf '${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}@sha256:%s ' *)

      - name: Inspect image
        run: |
          docker buildx imagetools inspect ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:${{ steps.meta.outputs.version }}

  merge-full:
    runs-on: ubuntu-latest
    needs: build
    permissions:
      contents: read
      packages: write
    steps:
      - name: Download digests
        uses: actions/download-artifact@v8
        with:
          path: /tmp/digests
          pattern: digests-*-full
          merge-multiple: true

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v4

      - name: Log in to GHCR
        uses: docker/login-action@v4
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Extract metadata
        id: meta
        uses: docker/metadata-action@v6
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
          tags: |
            type=ref,event=branch,suffix=-full
            type=semver,pattern={{version}},suffix=-full
            type=semver,pattern={{major}}.{{minor}},suffix=-full
            type=semver,pattern={{major}},suffix=-full

      - name: Create manifest list and push
        working-directory: /tmp/digests
        run: |
          docker buildx imagetools create \
            $(jq -cr '.tags | map("-t " + .) | join(" ")' <<< "$DOCKER_METADATA_OUTPUT_JSON") \
            $(printf '${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}@sha256:%s ' *)

      - name: Inspect image
        run: |
          docker buildx imagetools inspect ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:${{ steps.meta.outputs.version }}
```

- [ ] **Step 2: Validate YAML syntax**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/docker.yml'))"`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/docker.yml
git commit -m "feat: build slim (distroless) and full Docker variants in CI"
```

---

### Task 5: Run full test suite and verify

- [ ] **Step 1: Run the full Go test suite**

Run: `go test ./... -count=1`
Expected: All tests pass. No regressions from the build tag changes.

- [ ] **Step 2: Verify noimage build one final time**

Run: `CGO_ENABLED=0 go build -tags noimage -o /dev/null ./cmd/noda`
Expected: Compiles successfully.
