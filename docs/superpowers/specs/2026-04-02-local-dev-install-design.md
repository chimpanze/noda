# Local Dev Install — Design Spec

**Date:** 2026-04-02
**Status:** Draft

## Goal

Enable installing Noda on a fresh development machine with a single command. The binary is self-contained with the embedded editor UI, so the user only needs the `noda` binary in PATH to start developing APIs.

## Non-Goals

- Package manager distribution (Homebrew, apt, scoop) — future work
- `noda self-update` command — re-run the install script instead
- `go install` support — not viable because the editor must be embedded at build time

## Components

### 1. GoReleaser Configuration (`.goreleaser.yaml`)

**Build matrix:** 6 targets, all CGO-enabled (full variant with libvips):

| OS | Arch |
|----|------|
| darwin | amd64 |
| darwin | arm64 |
| linux | amd64 |
| linux | arm64 |
| windows | amd64 |
| windows | arm64 |

**Build hooks:** `before.hooks` runs the editor build (`npm ci && npm run build` in `editor/`, copy `editor/dist` to `editorfs/dist`).

**Ldflags:** Same as current Makefile — injects `Version`, `Commit`, `BuildTime` via GoReleaser template variables (`-X main.Version={{.Version}}`, etc.).

**Build tags:** `embed_editor` (always). No `noimage` tag — full variant for all targets.

**Archives:**
- `.tar.gz` for macOS/Linux
- `.zip` for Windows
- Naming: `noda_{{.Version}}_{{.Os}}_{{.Arch}}` (e.g., `noda_1.0.0_darwin_arm64.tar.gz`)
- Contents: `noda` binary, `LICENSE`, `README.md`

**Checksums:** Auto-generated `checksums.txt` (SHA256).

**Changelog:** Auto-generated from commit messages between tags.

### 2. GitHub Actions Workflow (`.github/workflows/release.yml`)

**Trigger:** `on: push: tags: ['v*']`

**Three parallel build jobs:**

| Job | Runner | Targets | libvips install |
|-----|--------|---------|-----------------|
| `build-linux` | `ubuntu-latest` | linux/amd64, linux/arm64 | `apt-get install libvips-dev` + arm64 cross-compile via `aarch64-linux-gnu-gcc` and `dpkg --add-architecture arm64` |
| `build-darwin` | `macos-latest` | darwin/amd64, darwin/arm64 | `brew install vips` |
| `build-windows` | `windows-latest` | windows/amd64, windows/arm64 | Prebuilt libvips binaries from the [libvips releases page](https://github.com/libvips/libvips/releases) (includes Windows amd64 + arm64 zips). Set `PKG_CONFIG_PATH` and `CGO_CFLAGS`/`CGO_LDFLAGS` to point at the extracted headers and libs. |

Each job:
1. Checks out code
2. Sets up Node.js 22 + Go 1.25
3. Installs libvips for the platform
4. Runs `goreleaser build --split`
5. Uploads build artifacts

**Cross-compilation notes:**
- **Linux arm64** from amd64 runner: `aarch64-linux-gnu-gcc` cross-compiler + arm64 libvips-dev via multiarch.
- **macOS:** `macos-latest` is arm64. Go cross-compiles to amd64 natively; libvips from Homebrew works for both.
- **Windows arm64** from amd64 runner: Go handles the cross-compile; libvips arm64 binaries sourced separately. If this proves too painful, fall back to slim (CGO_ENABLED=0, `noimage` tag) for windows/arm64 only.

**Merge job:** Runs after all three build jobs complete. Downloads all artifacts, runs `goreleaser continue --merge` to create the GitHub Release with all archives, checksums, and changelog.

### 3. Install Script (`install.sh`)

**Scope:** macOS and Linux only. Windows users download manually from the releases page.

**Steps:**
1. Detect OS (`uname -s` → `darwin` / `linux`). Exit with message if unsupported.
2. Detect arch (`uname -m` → map `x86_64` to `amd64`, `aarch64`/`arm64` to `arm64`).
3. Determine version: use `VERSION` env var if set, otherwise fetch latest from GitHub API (`/repos/chimpanze/noda/releases/latest`).
4. Download the archive and `checksums.txt` from the release.
5. Verify SHA256 checksum.
6. Extract binary from archive.
7. Install to `/usr/local/bin/noda` (with `sudo` if needed). Fallback to `~/.local/bin/noda` if no sudo access.
8. If installed to `~/.local/bin`, warn if it's not in `$PATH`.
9. Verify installation with `noda version`.

**Updates:** Re-running the script overwrites the existing binary. No version comparison — always installs the requested (or latest) version.

**Version pinning:** `curl -fsSL https://raw.githubusercontent.com/chimpanze/noda/main/install.sh | VERSION=v1.2.0 sh`

**Usage:**
```bash
curl -fsSL https://raw.githubusercontent.com/chimpanze/noda/main/install.sh | sh
```

### 4. Documentation Updates

**`docs/01-getting-started/installation.md`:**
- Primary method: curl one-liner install script
- Windows: manual download from releases page + instructions for adding to PATH
- Docker: stays as-is
- `go install`: removed (doesn't support embedded editor)

**Release page body:** GoReleaser changelog config includes a header with install instructions and the Windows manual download note.

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Windows arm64 + libvips too painful | Fall back to slim variant for windows/arm64 only |
| libvips version differences across runners | Pin libvips version in CI where possible |
| `macos-latest` runner arch changes | GoReleaser split handles this — builds both amd64 and arm64 regardless of host |
| GitHub API rate limits in install script | Use unauthenticated API (60 req/hr) — sufficient for install scripts; document `GITHUB_TOKEN` env var for CI use |
