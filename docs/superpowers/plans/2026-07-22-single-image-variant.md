# Single Build Variant Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close #425 by deleting every `CGO_ENABLED=0` build target — the Docker `slim` variant, the Windows release binary, and the `noimage` build tag — so Noda ships exactly one build configuration.

**Architecture:** This is a deletion, not a feature. `internal/dberr/sqlite.go` is left untouched: its dependency on cgo-only `mattn/go-sqlite3` constants becomes correct-by-construction once every build target enables cgo. The work is five independent deletions across Go source, the Dockerfile, two workflows, and docs. This plan effectively reverts `docs/superpowers/plans/2026-04-02-distroless-container.md`, which introduced the two-variant setup.

**Tech Stack:** Go 1.26 (cgo), Docker Buildx, GitHub Actions, libvips/bimg.

**Spec:** `docs/superpowers/specs/2026-07-22-single-image-variant-design.md`

## Global Constraints

- The one and only build configuration is `CGO_ENABLED=1` with `-tags embed_editor` and libvips present. No task may introduce a second one.
- **Do not modify `internal/dberr/sqlite.go`.** It is correct as written.
- **`docker.yml` must keep its build-by-digest + merge-manifest topology, and arm64 must keep building on the native `ubuntu-24.04-arm` runner.** Collapsing the two build jobs into a single multi-platform `build-push-action` step would reintroduce cross-compilation, which was deliberately removed from this repo.
- **Do not add a `pull_request` trigger to `docker.yml`.** Triggers stay `push` to `main` plus `v*` tags.
- Published tags are unsuffixed only: `main`, `latest`, `0.0.8`, `0.0`, `0`. The `-full` suffix is never produced again.
- `CHANGELOG.md`, `REVIEW-FINDINGS-*.md`, and `.verification/**` are point-in-time records. Historical mentions of `noimage`, `slim`, or `VARIANT` in them must be left alone.
- Specs and plans under `docs/superpowers/` are gitignored but tracked by convention — stage them with `git add -f`.
- Work happens on branch `fix/425-single-image-variant`, which already exists and carries the spec commit (`09d3019`).

---

### Task 1: Delete the `noimage` build tag

Removes the build-tagged indirection that let the image plugin be compiled out. After this task, the image plugin is an ordinary member of the plugin list in both places that enumerate plugins.

**Files:**
- Modify: `plugins/all/all.go:32-34,38-58`
- Delete: `plugins/all/all_image.go`
- Delete: `plugins/all/all_noimage.go`
- Modify: `tools/docverify/groundtruth/main.go:37-38,50-62`
- Delete: `tools/docverify/groundtruth/plugins_image.go`
- Delete: `tools/docverify/groundtruth/plugins_noimage.go`
- Modify: `internal/registry/service_schema_audit_test.go:12-14`
- Test: `plugins/all/all_test.go` (add one test to the existing file)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `all.Core() []api.Plugin` and `all.All() []api.Plugin` keep their existing signatures and their current default-build contents. The package-level `var optional []api.Plugin` is deleted; nothing outside `plugins/all` referenced it. `tools/docverify/groundtruth`'s `allPlugins() []api.Plugin` keeps its signature; its `var optionalPlugins []api.Plugin` is deleted.

- [ ] **Step 1: Write the characterization test**

Append to `plugins/all/all_test.go`. This asserts the guarantee the task establishes — the image plugin is in every build. It follows the `seen[...]` spot-check style already used by `TestAllIsCorePlusServiceOnly` in the same file.

**Write no doc comment on this test.** Step 9's exit criterion is that the string `noimage` appears in no `*.go` file, and a comment explaining the test's history would name the deleted tag and trip that check. The human chose the grep over the comment.

```go
func TestCoreIncludesImagePlugin(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range all.Core() {
		seen[p.Name()] = true
	}
	assert.True(t, seen["image"], "core plugin image missing from all.Core()")
}
```

- [ ] **Step 2: Run the test — it should PASS**

```bash
go test ./plugins/all/ -run TestCoreIncludesImagePlugin -v
```

Expected: PASS. This is deliberate and is not a broken TDD cycle — under default build tags the `!noimage` `init()` already appends the plugin, so the behavior is unchanged by this task. The test exists to catch the specific way Step 3 can go wrong: deleting `all_image.go` without adding `&imageplugin.Plugin{}` to the literal. Confirm it passes now so that a later failure is unambiguously your regression, not a pre-existing one.

- [ ] **Step 3: Fold the image plugin into `plugins/all/all.go`**

Delete the `optional` var and its doc comment (lines 32-34), add the image import, add the plugin to the literal, and return `plugins` directly.

Add to the import block, in alphabetical position between `httpplugin` and `livekitplugin`:

```go
	imageplugin "github.com/chimpanze/noda/plugins/image"
```

Delete these three lines entirely:

```go
// optional holds plugins registered via build-tagged init() functions
// (image, gated on !noimage).
var optional []api.Plugin
```

In `Core()`, add `&imageplugin.Plugin{}` after `&emailplugin.Plugin{},` and change the return:

```go
		&httpplugin.Plugin{},
		&emailplugin.Plugin{},
		&imageplugin.Plugin{},
		&corews.Plugin{},
		&coresse.Plugin{},
		&corewasm.Plugin{},
		&coreoidc.Plugin{},
		&livekitplugin.Plugin{},
		&authplugin.Plugin{},
	}
	return plugins
}
```

Note: `api` stays imported — `Core()`, `ServiceOnly()`, and `All()` all return `[]api.Plugin`.

- [ ] **Step 4: Delete the two build-tagged files in `plugins/all`**

```bash
git rm plugins/all/all_image.go plugins/all/all_noimage.go
```

- [ ] **Step 5: Fold the image plugin into `tools/docverify/groundtruth/main.go`**

Add to that file's import block, in alphabetical position between `httpplugin` and `livekitplugin`:

```go
	imageplugin "github.com/chimpanze/noda/plugins/image"
```

Delete these two lines:

```go
// optionalPlugins is appended by the build-tagged plugins_image.go.
var optionalPlugins []api.Plugin
```

Rewrite `allPlugins()` to include the image plugin and return directly:

```go
func allPlugins() []api.Plugin {
	return []api.Plugin{
		&control.Plugin{}, &transform.Plugin{}, &util.Plugin{},
		&workflow.Plugin{}, &response.Plugin{}, &dbplugin.Plugin{},
		&cacheplugin.Plugin{}, &event.Plugin{}, &corestorage.Plugin{},
		&upload.Plugin{}, &httpplugin.Plugin{}, &emailplugin.Plugin{},
		&imageplugin.Plugin{}, &corews.Plugin{}, &coresse.Plugin{},
		&corewasm.Plugin{}, &coreoidc.Plugin{}, &livekitplugin.Plugin{},
		&authplugin.Plugin{}, &streamplugin.Plugin{},
		&pubsubplugin.Plugin{}, &storageplugin.Plugin{},
	}
}
```

- [ ] **Step 6: Delete the two build-tagged files in `tools/docverify/groundtruth`**

```bash
git rm tools/docverify/groundtruth/plugins_image.go tools/docverify/groundtruth/plugins_noimage.go
```

- [ ] **Step 7: Reword the stale comment in the audit test**

In `internal/registry/service_schema_audit_test.go`, replace these three lines:

```go
// Under default build tags the list includes the image plugin; a
// `-tags noimage` test run audits exactly what the noimage build registers,
// which is the point of consuming the runtime's own list.
```

with:

```go
// The list always includes the image plugin: there is one build
// configuration and libvips is always present (#425). Consuming the
// runtime's own list is the point — the audit cannot drift from what
// actually gets registered.
```

- [ ] **Step 8: Verify the build and the whole affected test surface**

```bash
go build ./... && go vet ./... && go test ./plugins/all/... ./internal/registry/... ./tools/... 2>&1 | tail -20
```

Expected: no build or vet output, and `ok` for every tested package. `TestCoreIncludesImagePlugin` and `TestAllIsCorePlusServiceOnly` both pass; `TestServiceConfigSchemaAudit` passes unchanged (the plugin set it sees is identical to the previous default build).

- [ ] **Step 9: Verify the tag is gone from Go source**

```bash
grep -rn "noimage" --include="*.go" . | grep -v node_modules
```

Expected: **no output.**

- [ ] **Step 10: Commit**

```bash
git add plugins/all/ tools/docverify/groundtruth/ internal/registry/service_schema_audit_test.go
git commit -m "refactor: delete the noimage build tag (#425)

The image plugin was appended by a \`!noimage\`-tagged init(), so a
\`-tags noimage\` build registered a different plugin set. With one build
configuration there is nothing to gate: fold it into the plugin literal in
both enumeration sites and drop the optional-plugin indirection."
```

---

### Task 2: Collapse the Dockerfile to one variant

**Files:**
- Modify: `Dockerfile` (full rewrite, 84 lines → 50)

**Interfaces:**
- Consumes: Task 1's guarantee that the default build registers the image plugin.
- Produces: an image built from `debian:bookworm-slim` with libvips, entrypoint `/noda`, running as user `noda`. Task 3 builds this file with no `--build-arg`.

- [ ] **Step 1: Rewrite the Dockerfile**

Replace the entire contents of `Dockerfile` with:

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
FROM golang:1.26-bookworm AS builder

WORKDIR /build

RUN apt-get update && apt-get install -y --no-install-recommends \
        libvips-dev \
        pkg-config \
    && rm -rf /var/lib/apt/lists/*

# Cache Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source and embed built editor assets
COPY . .
COPY --from=editor /editor/dist editorfs/dist

RUN CGO_ENABLED=1 go build -tags embed_editor -ldflags "\
        -X main.Version=$(git describe --tags --always --dirty 2>/dev/null || echo 0.0.1-dev) \
        -X main.Commit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown) \
        -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        -o /noda ./cmd/noda

# Runtime stage
FROM debian:bookworm-slim

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
```

What changed: `ARG VARIANT=slim` and both `ARG VARIANT` re-declarations are gone; the libvips install is unconditional; the build is a single unconditional `RUN`; the `runtime-slim` distroless stage and the `FROM runtime-${VARIANT}` selector are deleted; the runtime stage no longer needs an `AS runtime-full` name.

- [ ] **Step 2: Build the image**

```bash
docker build -t noda-verify . 2>&1 | tail -20
```

Expected: `naming to docker.io/library/noda-verify` and no `undefined: sqlite3.` errors. This takes several minutes — the Node stage and the cgo Go build are both slow. If Docker is not running, start Docker Desktop first.

- [ ] **Step 3: Smoke-test the built image**

```bash
docker run --rm noda-verify version
```

Expected: a version line (e.g. `noda 0.0.1-dev`), not a crash or an entrypoint error.

- [ ] **Step 4: Verify no variant machinery survives**

```bash
grep -n "VARIANT\|runtime-slim\|distroless\|CGO_ENABLED=0" Dockerfile
```

Expected: **no output.**

- [ ] **Step 5: Commit**

```bash
git add Dockerfile
git commit -m "build: collapse the Dockerfile to a single cgo variant (#425)

The slim variant built with CGO_ENABLED=0, which has not compiled since
internal/dberr landed. Drop it: libvips is always installed, the build is
always CGO_ENABLED=1 -tags embed_editor, and the only runtime is
debian:bookworm-slim. Four stages become three with no conditionals."
```

---

### Task 3: Collapse `docker.yml` to one variant

**Files:**
- Modify: `.github/workflows/docker.yml` (full rewrite, 162 lines → 105)

**Interfaces:**
- Consumes: the `Dockerfile` from Task 2, built with no `--build-arg`.
- Produces: unsuffixed GHCR tags only.

- [ ] **Step 1: Rewrite the workflow**

Replace the entire contents of `.github/workflows/docker.yml` with:

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
          - platform: linux/arm64
            runner: ubuntu-24.04-arm
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
          outputs: type=image,name=${{ env.REGISTRY }}/${{ env.IMAGE_NAME }},push-by-digest=true,name-canonical=true,push=true
          cache-from: type=gha,scope=${{ matrix.platform }}
          cache-to: type=gha,scope=${{ matrix.platform }},mode=max

      - name: Export digest
        run: |
          mkdir -p /tmp/digests
          digest="${{ steps.build.outputs.digest }}"
          touch "/tmp/digests/${digest#sha256:}"

      - name: Upload digest
        uses: actions/upload-artifact@v7
        with:
          name: digests-${{ matrix.runner }}
          path: /tmp/digests/*
          if-no-files-found: error
          retention-days: 1

  merge:
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
          pattern: digests-*
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
```

What changed: the matrix drops `variant` (4 jobs → 2); `build-args` is gone; cache scope and digest artifact name drop the `-${{ matrix.variant }}` suffix; `merge-slim` and `merge-full` become one `merge` job with `pattern: digests-*` and no `suffix=-full` on any tag. The digest-then-merge topology and the native `ubuntu-24.04-arm` runner are unchanged.

- [ ] **Step 2: Lint the workflow**

```bash
actionlint .github/workflows/docker.yml
```

Expected: **no output.** (`actionlint` is already installed at `~/go/bin/actionlint`.)

- [ ] **Step 3: Verify the variant dimension is gone**

```bash
grep -n "variant\|VARIANT\|-full\|merge-slim" .github/workflows/docker.yml
```

Expected: **no output.**

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/docker.yml
git commit -m "ci: build one image variant instead of four jobs (#425)

Matrix loses the variant dimension (4 jobs -> 2) and merge-slim/merge-full
collapse into one merge job publishing unsuffixed tags. The build-by-digest
topology and the native arm64 runner are deliberately unchanged."
```

---

### Task 4: Retire the Windows release target

The Windows binary was the last `CGO_ENABLED=0` target and is broken by the same `internal/dberr` cgo dependency — latently, since `release.yml` has not run since `v0.0.7` (2026-07-19, pre-`dberr`). `.goreleaser.yaml` is deleted in the same task because it is wired to nothing and exists only to promise a Windows build that will no longer happen.

**Files:**
- Modify: `.github/workflows/release.yml:182-237` (delete `build-windows` job), `:240` (`needs` list)
- Delete: `.goreleaser.yaml`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `release.yml` builds four artifacts across three jobs — `noda-linux-amd64` and `noda-linux-arm64` (from the two-way `goarch` matrix in `build-linux`), `noda-darwin-arm64`, and `noda-darwin-amd64`. That is one fewer than the five it produces today. The `release` job's `pattern: noda-*` download and `artifacts/*` glob are unchanged and need no edit.

- [ ] **Step 1: Confirm the Windows binary is in fact broken**

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags embed_editor,noimage -o /dev/null ./cmd/noda 2>&1 | head -5
```

Expected: `undefined: sqlite3.ErrConstraint` and friends. This documents *why* the job is being deleted rather than repaired. (If Task 1 is already committed the `noimage` tag is inert, which is harmless — Go ignores unknown build tags. The failure is caused by `CGO_ENABLED=0`, not by the tag.)

- [ ] **Step 2: Delete the `build-windows` job**

Remove the whole job from `.github/workflows/release.yml` — it begins with `  build-windows:` (currently line 182) and ends at the blank line before `  release:` (currently line 237). That is the `runs-on: windows-latest` block including its checkout, setup-go, setup-node, "Build editor assets", "Build binary (slim — no image plugin)", "Create archive", and "Upload archive" steps.

- [ ] **Step 3: Drop it from the `release` job's `needs`**

Change:

```yaml
    needs: [build-linux, build-darwin-arm64, build-darwin-amd64, build-windows]
```

to:

```yaml
    needs: [build-linux, build-darwin-arm64, build-darwin-amd64]
```

- [ ] **Step 4: Delete the dead goreleaser config**

```bash
git rm .goreleaser.yaml
```

- [ ] **Step 5: Verify**

```bash
actionlint .github/workflows/release.yml && grep -rn "windows\|goreleaser" .github/workflows/release.yml
```

Expected: `actionlint` silent, and the grep produces **no output**.

- [ ] **Step 6: Commit**

```bash
git add .github/workflows/release.yml .goreleaser.yaml
git commit -m "ci: drop the Windows release binary and dead goreleaser config (#425)

The Windows job was the last CGO_ENABLED=0 target and has been broken since
internal/dberr landed; it just had not run yet, since release.yml only fires
on v* tags. .goreleaser.yaml is referenced by no workflow, Makefile, or
script and promised a Windows target we no longer ship."
```

---

### Task 5: Update docs, installer, and CHANGELOG

**Files:**
- Modify: `docs/04-guides/deployment.md:69-82`
- Modify: `docs/01-getting-started/installation.md:19-33`
- Modify: `install.sh:48-50`
- Modify: `README.md:25`
- Modify: `CHANGELOG.md` (append to the existing `### Removed` under `[Unreleased]`, line 184)

**Interfaces:**
- Consumes: the final shape of `Dockerfile` (Task 2), `docker.yml` (Task 3), and `release.yml` (Task 4). Every statement below must match what those files actually do.
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Rewrite the deployment guide's Dockerfile section**

In `docs/04-guides/deployment.md`, replace everything from `### Dockerfile` through the paragraph ending `is **not** the active release path.` with:

````markdown
### Dockerfile

Use the repository's `Dockerfile` — it is a three-stage build:

1. A Node stage builds the embedded editor assets.
2. A `golang:1.26-bookworm` builder installs libvips and compiles the binary with `CGO_ENABLED=1 -tags embed_editor`.
3. The runtime is `debian:bookworm-slim` with libvips, running as a non-root user with a `HEALTHCHECK` against `/health/live`.

There is a single image variant. libvips is always present, so `image.*` nodes always work.

```bash
docker build -t my-noda-app .
```

Prebuilt images are published to GHCR by the tag-triggered `docker.yml` workflow as multi-arch (amd64 + arm64) manifests under unsuffixed tags: `latest`, `0.0.8`, `0.0`, `0`. Release binaries (Linux amd64/arm64, macOS arm64/amd64) come from the `release.yml` matrix on `v*` tags. There is no prebuilt Windows binary — see the [installation guide](../01-getting-started/installation.md#windows).
````

- [ ] **Step 2: Rewrite the installation guide's Windows section**

In `docs/01-getting-started/installation.md`, replace the `## Windows` section (the heading, the five numbered steps, the "Verify the installation" line and its fenced `noda version` block) with:

````markdown
## Windows

There is no prebuilt Windows binary. Noda requires cgo and libvips, so on Windows either run it in Docker (simplest) or build from source.

**Docker:**

```
docker pull ghcr.io/chimpanze/noda:latest
```

**From source:** install [Go 1.26+](https://go.dev/dl/), Node 22+, a C toolchain (e.g. [MSYS2](https://www.msys2.org/) mingw-w64), and [libvips](https://www.libvips.org/install.html), then:

```
git clone https://github.com/chimpanze/noda.git
cd noda
make build
```

This produces `dist/noda`. Verify it:

```
dist/noda version
```
````

- [ ] **Step 3: Update the installer's unsupported-OS message**

In `install.sh`, replace:

```sh
            echo "For Windows, download the binary manually from:" >&2
            echo "  https://github.com/${REPO}/releases/latest" >&2
```

with:

```sh
            echo "For Windows, use Docker or build from source — see:" >&2
            echo "  https://github.com/${REPO}/blob/main/docs/01-getting-started/installation.md#windows" >&2
```

- [ ] **Step 4: Strengthen the README prerequisite**

In `README.md`, replace line 25:

```markdown
- libvips (for image processing, included in Docker image)
```

with:

```markdown
- libvips (required to build; included in the Docker image)
```

- [ ] **Step 5: Add the CHANGELOG entries**

Append these four bullets to the existing `### Removed` list under `## [Unreleased]` in `CHANGELOG.md`, after the stream-plugin bullet:

```markdown
- **BREAKING:** the `slim` Docker image variant and the `-full` tag suffix. There is now one image — `debian:bookworm-slim` with libvips, built with cgo — published under unsuffixed tags (`latest`, `0.0.8`, `0.0`, `0`). Existing `-full` tags remain in GHCR but receive no new versions; pullers of the unsuffixed tag get the former `-full` image, which is larger (≈119 MB vs ≈36 MB compressed on amd64) and no longer distroless, and in exchange `image.*` nodes now work in every image. The slim variant built with `CGO_ENABLED=0` and had not compiled since `internal/dberr` landed (#425).
- **BREAKING:** the prebuilt Windows release binary. It was the last `CGO_ENABLED=0` build target and was broken by the same cgo dependency. On Windows, use the Docker image or build from source with libvips — see the [installation guide](docs/01-getting-started/installation.md#windows).
- The `noimage` build tag. With one build configuration there is nothing to gate: the image plugin is now an unconditional member of the plugin list, and libvips is a hard requirement to build from source.
- `.goreleaser.yaml`. It was referenced by no workflow, Makefile, or script — `release.yml` is and remains the release path — and described a target set that no longer matches what ships.
```

- [ ] **Step 6: Verify no stale variant references remain in live docs**

```bash
grep -rn "VARIANT\|noimage\|noda:.*-full\|distroless" README.md install.sh docs/01-getting-started/ docs/04-guides/ docs/02-config/ docs/03-nodes/ docs/05-examples/
```

Expected: **no output.** (`docs/superpowers/`, `CHANGELOG.md`, `REVIEW-FINDINGS-*.md`, and `.verification/` are excluded on purpose — they are point-in-time records.)

- [ ] **Step 7: Verify the Windows download link is gone from user-facing text**

```bash
grep -rn "releases/latest" install.sh docs/01-getting-started/installation.md
```

Expected: no hit that is about Windows. A macOS/Linux `releases/latest` reference elsewhere in `install.sh` is correct and must stay.

- [ ] **Step 8: Commit**

```bash
git add docs/04-guides/deployment.md docs/01-getting-started/installation.md install.sh README.md CHANGELOG.md
git commit -m "docs: one image variant, no Windows binary (#425)

Rewrite the deployment guide's Dockerfile section for the single variant,
replace the Windows binary-download instructions with Docker/from-source,
point install.sh at that section, and record all four removals in the
CHANGELOG as breaking."
```

---

## Final verification

Run after all five tasks. Do not claim completion without pasting this output.

- [ ] **Full build, vet, and test**

```bash
go build ./... && go vet ./... && make test 2>&1 | tail -20
```

Expected: clean build and vet; `make test` ends with `ok` lines and no `FAIL`.

- [ ] **Lint**

```bash
make lint 2>&1 | tail -20
```

Expected: no findings. If `golangci-lint` flags `gofmt` on a file you touched, run `make fmt` and amend — golangci's gofmt check has caught formatting that local `go vet` did not.

- [ ] **Both workflows lint**

```bash
actionlint .github/workflows/docker.yml .github/workflows/release.yml
```

Expected: no output.

- [ ] **Image builds and runs**

```bash
docker build -t noda-verify . && docker run --rm noda-verify version
```

Expected: a version line.

- [ ] **Nothing cgo-less survives anywhere**

```bash
grep -rn "CGO_ENABLED=0\|CGO_ENABLED: \"0\"\|noimage" \
  --include="*.go" --include="*.yml" --include="*.yaml" --include="Dockerfile" --include="*.sh" --include="Makefile" . \
  | grep -v node_modules | grep -v "^./docs/superpowers/"
```

Expected: **no output.**

- [ ] **Push and open the PR**

```bash
git push -u origin fix/425-single-image-variant
gh pr create --title "build!: ship one image variant, drop the slim/Windows cgo-less targets (#425)" --body "$(cat <<'EOF'
Closes #425.

`internal/dberr/sqlite.go` (from #419) references cgo-only `mattn/go-sqlite3` constants, so every `CGO_ENABLED=0` target stopped compiling. Rather than teach `internal/dberr` to build without cgo, this deletes the cgo-less configuration entirely — Noda now ships exactly one build configuration: `CGO_ENABLED=1 -tags embed_editor` with libvips.

Removed:
- The Docker `slim` variant and the `-full` tag suffix. One image, unsuffixed tags.
- The prebuilt Windows release binary — the last `CGO_ENABLED=0` target, broken latently since #419 (`release.yml` has not run since `v0.0.7`, which predates `internal/dberr`).
- The `noimage` build tag, now with no build target to gate.
- `.goreleaser.yaml`, wired to nothing and promising a Windows build.

`internal/dberr/sqlite.go` is unchanged: its cgo dependency is correct-by-construction once every target enables cgo.

**Breaking, stated plainly:** `ghcr.io/chimpanze/noda:latest` grows from ≈36 MB to ≈119 MB compressed (amd64) and stops being distroless — it becomes debian-based with a shell. `projects/homebase` pulls the unsuffixed tag and so switches base image on its next pull, gaining working `image.*` nodes. Windows users must use Docker or build from source.

Design: `docs/superpowers/specs/2026-07-22-single-image-variant-design.md`

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Confirm CI goes green**

```bash
gh pr checks --watch
```

Expected: the four required functional checks pass. Note that the benchmark check is **not** required. `docker.yml` does not run on PRs by design — the image build is only exercised on merge to `main`, which is the moment the currently-red Docker workflow should go green for the first time since 2026-07-21.

## Known flakes — a failure in these is probably noise, not your change

- `TestWatcher_Debounce` (`internal/devmode`) fails under CI load — issue #347.
- `TestEventHub_NoGoroutineLeakOnUnsubscribe` (`internal/trace`) fails at `-count>1` — issue #416.
- A lone `TestCookbook/livekit` failure is a known room-delete/egress race — issue #396.

Re-run rather than debugging, unless your change plausibly touched the area — none of these tasks do.
