# Distroless Container Build

## Goal

Reduce Docker image size and attack surface by building a distroless slim variant alongside the existing full variant, controlled by a single Dockerfile with a build arg.

## Motivation

- **Security**: The current `debian:bookworm-slim` runtime image includes a shell, package manager, and ~90+ packages. A distroless base eliminates all of that — no shell, no package manager, nothing to exploit.
- **Size**: The distroless static base is ~5MB vs ~80MB for debian-slim. Combined with the Go binary, the slim image should be under 30MB.

## Constraints

- The `bimg` image plugin requires CGO and dynamically links against `libvips`. A fully static binary cannot include it.
- Most deployments don't use the image plugin, so a slim variant without it is the sensible default.

## Design

### Single Dockerfile, Two Variants

A `VARIANT` build arg (default: `slim`) controls the build:

| Variant | CGO | libvips | Runtime Base | Image Size |
|---------|-----|---------|-------------|------------|
| `slim` | disabled | no | `gcr.io/distroless/static-debian12` | ~25MB |
| `full` | enabled | yes | `debian:bookworm-slim` | ~120MB |

### Dockerfile Structure

Three stages:

1. **Editor stage** (unchanged) — `node:22-bookworm-slim`, builds the React frontend.
2. **Builder stage** — `golang:1.25-bookworm`. Conditionally installs `libvips-dev` and sets `CGO_ENABLED` based on `VARIANT`.
3. **Runtime stage** — two `FROM` targets selected by variant:
   - `slim`: `gcr.io/distroless/static-debian12`. No user setup needed (runs as `nonroot` uid 65534 by default). No `HEALTHCHECK` (no `wget` available; use external probes).
   - `full`: `debian:bookworm-slim` with `libvips`, `ca-certificates`, `tzdata`, `wget`. Creates `noda` user. Includes `HEALTHCHECK` as today.

### CI Pipeline Changes

File: `.github/workflows/docker.yml`

The build matrix gains a `variant` dimension:

```yaml
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
```

**Tagging strategy:**
- Slim (default): `latest`, `v1.2.3`, `v1.2`, `v1`
- Full: `latest-full`, `v1.2.3-full`, `v1.2-full`, `v1-full`

Slim gets the clean tags as the recommended default.

**Artifact naming:** `digests-${{ matrix.runner }}-${{ matrix.variant }}` to avoid collisions.

**Merge job:** Split into two jobs (`merge-slim` and `merge-full`), each filtering artifacts by variant to create separate multi-arch manifest lists with the appropriate tags.

The `VARIANT` build arg is passed via the `build-args` input on `docker/build-push-action`.

### Healthcheck

- `full`: keeps the current `HEALTHCHECK` with `wget`.
- `slim`: no `HEALTHCHECK` directive. Users rely on Kubernetes liveness/readiness probes or docker-compose healthcheck config, which is the standard practice for distroless.

### User/Permissions

- `full`: creates `noda` user with `groupadd`/`useradd`, runs as `noda` (unchanged).
- `slim`: runs as the built-in `nonroot` user (uid 65534) provided by the distroless base.

### Build Tag for Image Plugin Exclusion

The `plugins/image` package imports `bimg`, which requires CGO. Building with `CGO_ENABLED=0` will fail unless the image plugin is excluded at compile time.

A `noimage` build tag conditionally excludes the image plugin, following the existing `embed_editor` pattern in the codebase.

**Two registration points need splitting:**

1. **`cmd/noda/`** — the image plugin import and `&imageplugin.Plugin{}` in the plugin list.
   - `cmd/noda/plugins_image.go` (`//go:build !noimage`) — adds the image plugin to the plugin list.
   - `cmd/noda/plugins_noimage.go` (`//go:build noimage`) — no-op stub.

2. **`internal/mcp/`** — the image plugin import and entry in `corePlugins()`.
   - `internal/mcp/plugins_image.go` (`//go:build !noimage`) — adds the image plugin.
   - `internal/mcp/plugins_noimage.go` (`//go:build noimage`) — no-op stub.

**Mechanism:** Each build-tagged file provides an `init()` function that appends `&imageplugin.Plugin{}` to a package-level slice. The `corePlugins()` functions (in `main.go` and `plugins.go`) read from this slice rather than hardcoding the image plugin entry. The `noimage` variant files are empty stubs that register nothing.

**Dockerfile integration:** The slim variant builds with `-tags noimage,embed_editor` and `CGO_ENABLED=0`. The full variant builds with `-tags embed_editor` and `CGO_ENABLED=1` (current behavior).

## Testing

### Go code changes

1. **Default build** — `go build ./cmd/noda` (no `noimage` tag) still compiles with CGO and includes the image plugin.
2. **Noimage build** — `CGO_ENABLED=0 go build -tags noimage ./cmd/noda` compiles successfully as a static binary.
3. **Full test suite** — `go test ./...` still passes without regressions (tests always run under the default build, never with `noimage`).

### Docker changes

5. **Local smoke test** — build both variants (`docker build --build-arg VARIANT=slim` and `VARIANT=full`), verify they start and respond on `/health/live`.
6. **Image inspection** — verify the slim image has no shell (`docker run --entrypoint sh` should fail), verify the full image still has libvips.
7. **CI validation** — the pipeline builds and pushes both variants; build failures are caught by the existing job structure.

## Files Changed

- `cmd/noda/plugins_image.go` — new, `//go:build !noimage`, registers image plugin
- `cmd/noda/plugins_noimage.go` — new, `//go:build noimage`, no-op stub
- `cmd/noda/main.go` — remove hardcoded image plugin import and registration
- `internal/mcp/plugins_image.go` — new, `//go:build !noimage`, registers image plugin
- `internal/mcp/plugins_noimage.go` — new, `//go:build noimage`, no-op stub
- `internal/mcp/plugins.go` — remove hardcoded image plugin import and registration
- `Dockerfile` — restructure with `VARIANT` build arg, conditional stages
- `.github/workflows/docker.yml` — add variant to matrix, update tagging and artifact names, split merge job by variant
