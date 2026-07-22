# Single build variant: drop the slim image and the Windows binary

**Date:** 2026-07-22
**Issue:** #425 — Docker slim image has not built since #419
**Status:** Approved, ready for planning

## Problem

`internal/dberr/sqlite.go` (added by #419, commit `360b4e6`) references `mattn/go-sqlite3`
constants that live behind `//go:build cgo`. Every build with `CGO_ENABLED=0` therefore
fails to compile:

```
CGO_ENABLED=0 go build -tags noimage,embed_editor ./internal/dberr/
internal/dberr/sqlite.go:34:37: undefined: sqlite3.ErrConstraint
internal/dberr/sqlite.go:36:31: undefined: sqlite3.ErrNoExtended
(…)
```

Three shipped build targets use `CGO_ENABLED=0`, and all three are broken:

| Target | Where | Status |
|---|---|---|
| Docker `slim`, linux/amd64 | `Dockerfile` + `docker.yml` | red on every push to `main` since 2026-07-21 |
| Docker `slim`, linux/arm64 | `Dockerfile` + `docker.yml` | red on every push to `main` since 2026-07-21 |
| Windows release binary | `release.yml:209-221` | broken but latent — has not run since `v0.0.7` (2026-07-19, pre-`dberr`) |

The `full` Docker jobs (`CGO_ENABLED=1`) are red only because matrix fail-fast cancels
them; their logs show `The operation was canceled` with no compile errors.

Last green Docker run: `598e16f`, the direct parent of `360b4e6`. `main` and `main-full`
images have been stale since 2026-07-21 and reflect none of #419/#420/#421/#422/#424.
Released images are so far unaffected — `latest`/`0.0.7` point at `v0.0.7` = `3c32ad7`,
which predates `internal/dberr` — but the next `v*` tag would publish an incomplete set.

### Why nothing caught it

1. CI never builds with `CGO_ENABLED=0`. The `go` job builds with cgo on by default, so
   `go build ./...` passes. The cgo-less configuration existed only inside the Dockerfile
   and the Windows release job.
2. `docker.yml` triggers only on `push` to `main` and `v*` tags, never on `pull_request`,
   so no PR check could go red. #419 was green on its PR and broke `main` on merge.

## Decision

Rather than teach `internal/dberr` to compile without cgo, **delete the cgo-less
configuration entirely.** Noda ships exactly one build configuration:

```
CGO_ENABLED=1  -tags embed_editor   (libvips present)
```

This closes #425 by removing the broken targets, not by working around the breakage. It
also removes the *class* of bug: an untested build configuration that rots silently.

### Decisions taken during design

| Question | Decision |
|---|---|
| The Windows binary is the last `CGO_ENABLED=0` target | **Drop it.** No build-tag split of `internal/dberr` is needed; that file stays as written. |
| Tag scheme once one variant remains | **Unsuffixed only** — `main`, `latest`, `0.0.8`, `0.0`, `0`. The `-full` suffix stops being produced; existing `-full` tags stay in GHCR as history. |
| Fate of the now-unused `noimage` build tag | **Delete it entirely.** libvips-dev becomes a hard requirement to build from source. |
| Add a `pull_request` trigger to `docker.yml` | **No.** With one configuration, the image compiles exactly what CI's `go` job already compiles, so a Go compile break can no longer reach `main` through the image. Only a Dockerfile-level mistake could, which is a narrower risk than the CI cost. |
| Dead `.goreleaser.yaml` | **Delete the file.** |

## Design

### 1. Dockerfile

Remove `ARG VARIANT` and both `if [ "$VARIANT" = "full" ]` branches.

- Builder unconditionally installs `libvips-dev` and `pkg-config`.
- Build is unconditionally `CGO_ENABLED=1 go build -tags embed_editor` with the existing
  `-ldflags` version stamping.
- One runtime stage: `debian:bookworm-slim` with libvips, ca-certificates, tzdata, wget;
  the `noda` non-root user; the `/health/live` `HEALTHCHECK`; `ENTRYPOINT ["/noda"]`.
- The `runtime-slim` distroless stage and the `FROM runtime-${VARIANT}` selector are
  deleted.

Result: four stages become three (editor, builder, runtime) with no conditionals.

### 2. `.github/workflows/docker.yml`

- Matrix loses the `variant` dimension: four jobs become two — `linux/amd64` on
  `ubuntu-latest`, `linux/arm64` on `ubuntu-24.04-arm`.
- **The build-by-digest + merge-manifest topology is preserved.** arm64 must keep
  building on a native arm64 runner; do not collapse this into a single multi-platform
  `build-push-action` step, which would reintroduce cross-compilation.
- Drop the `build-args: VARIANT=...` input.
- Cache scope becomes `type=gha,scope=${{ matrix.platform }}`.
- Digest artifact name becomes `digests-${{ matrix.runner }}`.
- `merge-slim` and `merge-full` collapse into one `merge` job that downloads
  `pattern: digests-*` and emits unsuffixed tags only:
  `type=ref,event=branch`, `type=semver,pattern={{version}}`,
  `type=semver,pattern={{major}}.{{minor}}`, `type=semver,pattern={{major}}`.
- Triggers are unchanged (`push` to `main`, `v*` tags).

### 3. `.github/workflows/release.yml`

- Delete the `build-windows` job in its entirety (currently lines 182–237, ending where
  the `release` job begins).
- Remove `build-windows` from the `release` job's `needs` list.
- No other change: the `release` job downloads with `pattern: noda-*` and globs
  `artifacts/*`, so it simply collects four artifacts instead of five
  (`build-linux` is itself a two-way `goarch` matrix).

### 4. Delete the `noimage` build tag

`plugins/all/`:
- Delete `all_image.go` and `all_noimage.go`.
- Add `&imageplugin.Plugin{}` to the `Core()` plugin literal in `all.go`.
- Delete the `optional` package var, its doc comment, and change the return from
  `append(plugins, optional...)` to `plugins`.

`tools/docverify/groundtruth/`:
- Delete `plugins_image.go` and `plugins_noimage.go`.
- Add the image plugin to the list in `main.go`; delete the `optionalPlugins` var, its
  doc comment, and the `append(plugins, optionalPlugins...)` return.

`internal/registry/service_schema_audit_test.go`:
- Reword the doc comment sentence describing a `-tags noimage` test run. This file carries
  no build tag; the reference is prose only.

### 5. Delete `.goreleaser.yaml`

Referenced by no workflow, Makefile, or script — confirmed by grep. It is dead config that
contradicts the shipped target set in two places: a `windows` goos, and a `release.header`
instructing Windows users to download a `.zip`.

### 6. Docs and user-facing text

- `docs/04-guides/deployment.md:73-80` — rewrite the Dockerfile section: one image, no
  `VARIANT` build-arg, and a note that libvips is always present so `image.*` nodes always
  work. Two further edits in the paragraph below it: drop `Windows` from the list of
  release-binary platforms, and delete the trailing clause stating that `.goreleaser.yaml`
  is not the active release path, since that file no longer exists.
- `docs/01-getting-started/installation.md:19-33` — replace the Windows binary-download
  steps with build-from-source instructions, stating the libvips prerequisite.
- `install.sh:49` — the "download the binary manually" message points to build-from-source
  instead of the releases page.
- Add a build-from-source prerequisite note (libvips-dev is now mandatory; there is no
  longer a `noimage` escape hatch) wherever building from source is documented.
- `CHANGELOG.md` under `[Unreleased]`, marked **BREAKING**: the slim image and `-full` tags
  are discontinued; the Windows binary is discontinued.

## Consequences

Accepted knowingly:

- **`ghcr.io/chimpanze/noda:latest` grows from 36 MB to 119 MB compressed (amd64)** and
  stops being distroless — it becomes debian-based with a shell. This is a real
  attack-surface increase on the image running in production.
- **`projects/homebase` switches base image on its next pull.** Its compose file pulls the
  unsuffixed tag (`ghcr.io/chimpanze/noda:${NODA_VERSION:-0.0.7}`), which now resolves to
  the debian+libvips image. No compose edit is required; the change is silent and
  intentional.
- **Windows users lose the prebuilt binary** and must build from source with libvips, which
  is awkward on Windows.
- Anyone pinned to a `-full` tag keeps working on existing tags but receives no new ones.

## Verification

1. `go build ./...` and `go vet ./...` clean.
2. `make test` passes.
3. `docker build -t noda-verify .` succeeds locally (native arm64), and
   `docker run --rm noda-verify version` prints a version.
4. Grep proves `noimage`, `VARIANT`, and `runtime-slim` appear nowhere outside
   `CHANGELOG.md` and the historical `REVIEW-FINDINGS-*.md` / `.verification/` files, which
   are point-in-time records and are not updated.
5. On merge to `main`, the Docker workflow goes green for the first time since
   2026-07-21 and republishes `main`.

## Out of scope

- Any change to `internal/dberr/sqlite.go`. Its cgo dependency becomes correct-by-
  construction once every build target enables cgo.
- #415 (build-tagged *test* files escaping golangci-lint). Deleting `noimage` reduces the
  build-tag surface but does not address that issue.
- Re-pinning `projects/homebase` to a new `NODA_VERSION`.
