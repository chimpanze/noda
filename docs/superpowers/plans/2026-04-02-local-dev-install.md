# Local Dev Install Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable `curl -fsSL .../install.sh | sh` to install a self-contained Noda binary (with embedded editor and libvips) on macOS/Linux, backed by GoReleaser + GitHub Actions CI.

**Architecture:** GoReleaser split/merge strategy — three parallel GitHub Actions jobs build on native runners (macOS, Linux, Windows) to handle CGO cross-compilation with libvips. A merge job combines artifacts into a single GitHub Release. A POSIX shell install script downloads and installs the correct binary.

**Tech Stack:** GoReleaser v2, GitHub Actions, POSIX shell, SHA256 checksums

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `.goreleaser.yaml` | GoReleaser build/archive/release config |
| Create | `.github/workflows/release.yml` | CI workflow: build on 3 OS runners, merge release |
| Create | `install.sh` | POSIX install script for macOS/Linux |
| Modify | `docs/01-getting-started/installation.md` | Updated install instructions |
| Modify | `Makefile` | Add `make install` target for local dev builds |

---

### Task 1: GoReleaser Configuration

**Files:**
- Create: `.goreleaser.yaml`

- [ ] **Step 1: Create `.goreleaser.yaml`**

```yaml
# yaml-language-server: $schema=https://goreleaser.com/static/schema.json
version: 2

before:
  hooks:
    - cmd: npm ci
      dir: editor
    - cmd: npm run build
      dir: editor
    - cmd: sh -c "rm -rf editorfs/dist && cp -r editor/dist editorfs/dist"

builds:
  - id: noda
    main: ./cmd/noda
    binary: noda
    env:
      - CGO_ENABLED=1
    flags:
      - -tags=embed_editor
    ldflags:
      - -X main.Version={{.Version}}
      - -X main.Commit={{.ShortCommit}}
      - -X main.BuildTime={{.Date}}
    goos:
      - darwin
      - linux
      - windows
    goarch:
      - amd64
      - arm64

archives:
  - id: noda
    builds:
      - noda
    name_template: "noda_{{.Version}}_{{.Os}}_{{.Arch}}"
    format_overrides:
      - goos: windows
        format: zip
    files:
      - LICENSE
      - README.md

checksum:
  name_template: "checksums.txt"
  algorithm: sha256

changelog:
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - "^ci:"
      - "^chore:"

release:
  github:
    owner: chimpanze
    name: noda
  header: |
    ## Install

    **macOS / Linux:**
    ```bash
    curl -fsSL https://raw.githubusercontent.com/chimpanze/noda/main/install.sh | sh
    ```

    **Windows:** Download the `.zip` for your architecture below, extract `noda.exe`, and add its directory to your `PATH`.
    See the [installation docs](https://github.com/chimpanze/noda/blob/main/docs/01-getting-started/installation.md) for details.
```

- [ ] **Step 2: Validate the config locally**

Run: `goreleaser check`
Expected: `config is valid`

If goreleaser is not installed, install it first:
```bash
go install github.com/goreleaser/goreleaser/v2@latest
```

- [ ] **Step 3: Commit**

```bash
git add .goreleaser.yaml
git commit -m "build: add GoReleaser configuration for cross-platform releases"
```

---

### Task 2: GitHub Actions Release Workflow

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Create `.github/workflows/release.yml`**

```yaml
name: Release

on:
  push:
    tags: ["v*"]

permissions:
  contents: write

jobs:
  build-linux:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
        with:
          fetch-depth: 0

      - name: Install libvips (amd64 + arm64 cross-compile)
        run: |
          sudo dpkg --add-architecture arm64
          # Add arm64 package sources
          sudo sed -i 's/^deb /deb [arch=amd64] /' /etc/apt/sources.list.d/*.list
          echo "deb [arch=arm64] http://ports.ubuntu.com/ubuntu-ports $(lsb_release -cs) main restricted universe multiverse" | sudo tee /etc/apt/sources.list.d/arm64.list
          echo "deb [arch=arm64] http://ports.ubuntu.com/ubuntu-ports $(lsb_release -cs)-updates main restricted universe multiverse" | sudo tee -a /etc/apt/sources.list.d/arm64.list
          sudo apt-get update
          sudo apt-get install -y \
            libvips-dev \
            libvips-dev:arm64 \
            gcc-aarch64-linux-gnu \
            g++-aarch64-linux-gnu

      - uses: actions/setup-go@v6
        with:
          go-version: "1.25"
          cache: true

      - uses: actions/setup-node@v6
        with:
          node-version: "22"
          cache: "npm"
          cache-dependency-path: editor/package-lock.json

      - name: GoReleaser build (split)
        uses: goreleaser/goreleaser-action@v6
        with:
          version: "~> v2"
          args: build --split
        env:
          GOOS: linux
          CC_linux_arm64: aarch64-linux-gnu-gcc
          CXX_linux_arm64: aarch64-linux-gnu-g++
          PKG_CONFIG_PATH_linux_arm64: /usr/lib/aarch64-linux-gnu/pkgconfig

      - name: Upload artifacts
        uses: actions/upload-artifact@v7
        with:
          name: build-linux
          path: dist/**/*
          retention-days: 1

  build-darwin:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v6
        with:
          fetch-depth: 0

      - name: Install libvips
        run: brew install vips

      - uses: actions/setup-go@v6
        with:
          go-version: "1.25"
          cache: true

      - uses: actions/setup-node@v6
        with:
          node-version: "22"
          cache: "npm"
          cache-dependency-path: editor/package-lock.json

      - name: GoReleaser build (split)
        uses: goreleaser/goreleaser-action@v6
        with:
          version: "~> v2"
          args: build --split
        env:
          GOOS: darwin

      - name: Upload artifacts
        uses: actions/upload-artifact@v7
        with:
          name: build-darwin
          path: dist/**/*
          retention-days: 1

  build-windows:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v6
        with:
          fetch-depth: 0

      - name: Install libvips
        shell: pwsh
        run: |
          $VIPS_VERSION = "8.16.1"
          $VIPS_URL = "https://github.com/libvips/libvips/releases/download/v${VIPS_VERSION}/vips-dev-w64-all-${VIPS_VERSION}.zip"
          Invoke-WebRequest -Uri $VIPS_URL -OutFile vips.zip
          Expand-Archive vips.zip -DestinationPath C:\vips
          $vipsDir = Get-ChildItem C:\vips -Directory | Select-Object -First 1
          echo "PKG_CONFIG_PATH=$($vipsDir.FullName)\lib\pkgconfig" >> $env:GITHUB_ENV
          echo "CGO_CFLAGS=-I$($vipsDir.FullName)\include" >> $env:GITHUB_ENV
          echo "CGO_LDFLAGS=-L$($vipsDir.FullName)\lib" >> $env:GITHUB_ENV
          echo "$($vipsDir.FullName)\bin" >> $env:GITHUB_PATH

      - uses: actions/setup-go@v6
        with:
          go-version: "1.25"
          cache: true

      - uses: actions/setup-node@v6
        with:
          node-version: "22"
          cache: "npm"
          cache-dependency-path: editor/package-lock.json

      - name: GoReleaser build (split)
        uses: goreleaser/goreleaser-action@v6
        with:
          version: "~> v2"
          args: build --split
        env:
          GOOS: windows

      - name: Upload artifacts
        uses: actions/upload-artifact@v7
        with:
          name: build-windows
          path: dist/**/*
          retention-days: 1

  merge:
    runs-on: ubuntu-latest
    needs: [build-linux, build-darwin, build-windows]
    steps:
      - uses: actions/checkout@v6
        with:
          fetch-depth: 0

      - name: Download all artifacts
        uses: actions/download-artifact@v8
        with:
          path: dist
          pattern: build-*
          merge-multiple: true

      - name: GoReleaser merge and release
        uses: goreleaser/goreleaser-action@v6
        with:
          version: "~> v2"
          args: continue --merge
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

- [ ] **Step 2: Validate workflow syntax**

Run: `actionlint .github/workflows/release.yml`

If actionlint is not installed:
```bash
go install github.com/rhysd/actionlint/cmd/actionlint@latest
```

Expected: no errors (warnings about expressions are OK).

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add release workflow with GoReleaser split/merge"
```

---

### Task 3: Install Script

**Files:**
- Create: `install.sh`

- [ ] **Step 1: Create `install.sh`**

```bash
#!/bin/sh
set -eu

REPO="chimpanze/noda"
INSTALL_DIR="/usr/local/bin"
FALLBACK_DIR="${HOME}/.local/bin"

main() {
    check_dependencies

    os=$(detect_os)
    arch=$(detect_arch)

    version=$(resolve_version)
    echo "Installing noda ${version} (${os}/${arch})..."

    tmpdir=$(mktemp -d)
    trap 'rm -rf "${tmpdir}"' EXIT

    download "${version}" "${os}" "${arch}" "${tmpdir}"
    verify_checksum "${tmpdir}" "${os}" "${arch}" "${version}"
    install_binary "${tmpdir}" "${os}" "${arch}" "${version}"

    echo ""
    noda version
    echo ""
    echo "noda installed successfully."
}

check_dependencies() {
    for cmd in curl sha256sum tar; do
        if ! command -v "${cmd}" >/dev/null 2>&1; then
            # macOS uses shasum instead of sha256sum
            if [ "${cmd}" = "sha256sum" ] && command -v shasum >/dev/null 2>&1; then
                continue
            fi
            echo "Error: '${cmd}' is required but not found." >&2
            exit 1
        fi
    done
}

detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        *)
            echo "Error: Unsupported operating system '$(uname -s)'." >&2
            echo "For Windows, download the binary manually from:" >&2
            echo "  https://github.com/${REPO}/releases/latest" >&2
            exit 1
            ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)   echo "amd64" ;;
        aarch64|arm64)   echo "arm64" ;;
        *)
            echo "Error: Unsupported architecture '$(uname -m)'." >&2
            exit 1
            ;;
    esac
}

resolve_version() {
    if [ -n "${VERSION:-}" ]; then
        echo "${VERSION}"
        return
    fi

    version=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
        | grep '"tag_name"' \
        | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')

    if [ -z "${version}" ]; then
        echo "Error: Could not determine latest version." >&2
        echo "Set VERSION env var to install a specific version, e.g.:" >&2
        echo "  VERSION=v1.0.0 curl -fsSL ... | sh" >&2
        exit 1
    fi

    echo "${version}"
}

download() {
    version="$1"; os="$2"; arch="$3"; dir="$4"
    # Version without leading 'v' for archive name
    ver="${version#v}"
    archive="noda_${ver}_${os}_${arch}.tar.gz"
    url="https://github.com/${REPO}/releases/download/${version}/${archive}"
    checksums_url="https://github.com/${REPO}/releases/download/${version}/checksums.txt"

    echo "Downloading ${url}..."
    curl -fsSL -o "${dir}/${archive}" "${url}"
    curl -fsSL -o "${dir}/checksums.txt" "${checksums_url}"
}

verify_checksum() {
    dir="$1"; os="$2"; arch="$3"; version="$4"
    ver="${version#v}"
    archive="noda_${ver}_${os}_${arch}.tar.gz"

    expected=$(grep "${archive}" "${dir}/checksums.txt" | awk '{print $1}')
    if [ -z "${expected}" ]; then
        echo "Error: Checksum not found for ${archive}." >&2
        exit 1
    fi

    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "${dir}/${archive}" | awk '{print $1}')
    else
        actual=$(shasum -a 256 "${dir}/${archive}" | awk '{print $1}')
    fi

    if [ "${actual}" != "${expected}" ]; then
        echo "Error: Checksum mismatch." >&2
        echo "  Expected: ${expected}" >&2
        echo "  Actual:   ${actual}" >&2
        exit 1
    fi

    echo "Checksum verified."
}

install_binary() {
    dir="$1"; os="$2"; arch="$3"; version="$4"
    ver="${version#v}"
    archive="noda_${ver}_${os}_${arch}.tar.gz"

    tar -xzf "${dir}/${archive}" -C "${dir}"

    # Try /usr/local/bin first, fall back to ~/.local/bin
    if [ -w "${INSTALL_DIR}" ]; then
        cp "${dir}/noda" "${INSTALL_DIR}/noda"
        chmod +x "${INSTALL_DIR}/noda"
        echo "Installed to ${INSTALL_DIR}/noda"
    elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
        sudo cp "${dir}/noda" "${INSTALL_DIR}/noda"
        sudo chmod +x "${INSTALL_DIR}/noda"
        echo "Installed to ${INSTALL_DIR}/noda (via sudo)"
    else
        mkdir -p "${FALLBACK_DIR}"
        cp "${dir}/noda" "${FALLBACK_DIR}/noda"
        chmod +x "${FALLBACK_DIR}/noda"
        echo "Installed to ${FALLBACK_DIR}/noda"
        case ":${PATH}:" in
            *":${FALLBACK_DIR}:"*) ;;
            *)
                echo ""
                echo "WARNING: ${FALLBACK_DIR} is not in your PATH."
                echo "Add it by running:"
                echo "  export PATH=\"${FALLBACK_DIR}:\$PATH\""
                echo "Add the line above to your ~/.bashrc, ~/.zshrc, or equivalent."
                ;;
        esac
    fi
}

main
```

- [ ] **Step 2: Make the script executable**

Run: `chmod +x install.sh`

- [ ] **Step 3: Test the script's detection logic locally**

Run: `sh -c 'set -eu; . ./install.sh; detect_os; detect_arch'`

This won't install anything — it verifies the OS/arch detection functions parse correctly on your machine. Expected output (on macOS Apple Silicon):
```
darwin
arm64
```

Note: The full script can't be end-to-end tested until a release exists. We'll validate it after the first tag push.

- [ ] **Step 4: Commit**

```bash
git add install.sh
git commit -m "feat: add install script for curl-pipe-sh installation"
```

---

### Task 4: Update Installation Documentation

**Files:**
- Modify: `docs/01-getting-started/installation.md`

- [ ] **Step 1: Rewrite installation.md**

Replace the full contents of `docs/01-getting-started/installation.md` with:

```markdown
# Installation

Noda is a configuration-driven API runtime for Go. You define routes, workflows, middleware, auth, services, and real-time connections in JSON config files — no application code required for standard patterns. Custom logic runs in Wasm modules.

## Quick Install (macOS / Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/chimpanze/noda/main/install.sh | sh
```

This downloads the latest release binary and installs it to `/usr/local/bin`. To install a specific version:

```bash
curl -fsSL https://raw.githubusercontent.com/chimpanze/noda/main/install.sh | VERSION=v1.0.0 sh
```

Re-run the same command to update to the latest version.

## Windows

1. Download the `.zip` for your architecture from the [latest release](https://github.com/chimpanze/noda/releases/latest)
2. Extract `noda.exe` to a directory of your choice (e.g., `C:\Program Files\noda\`)
3. Add that directory to your system PATH:
   - Open **Settings > System > About > Advanced system settings**
   - Click **Environment Variables**
   - Under **System variables**, select `Path` and click **Edit**
   - Click **New** and add the directory containing `noda.exe`
   - Click **OK** to save

Verify the installation:

```
noda version
```

## Docker

```bash
docker pull ghcr.io/chimpanze/noda:latest
```

## Prerequisites

- **PostgreSQL** (optional) — for database operations
- **Redis** (optional) — for caching, events, pub/sub, distributed locking
- **libvips** (optional) — for image processing (`image.*` nodes). The prebuilt binary includes libvips, but if building from source you need it installed.

## CLI Reference

| Command | Description |
|---------|-------------|
| `noda init [name]` | Scaffold a new project |
| `noda start` | Start the production server |
| `noda dev` | Start in dev mode with hot reload |
| `noda validate` | Validate all config files |
| `noda test` | Run workflow tests |
| `noda migrate create [name]` | Create a new migration |
| `noda migrate up` | Apply all pending migrations |
| `noda migrate down` | Roll back the last migration |
| `noda migrate status` | Show migration status |
| `noda generate openapi` | Generate OpenAPI 3.1 specification |
| `noda schedule status` | Show configured scheduled jobs |
| `noda plugin list` | List all registered plugins and node counts |
| `noda mcp` | Start MCP server for AI agent integration |
| `noda version` | Print version and build info |
| `noda completion <shell>` | Generate shell completions |

### Global Flags

| Flag | Description |
|------|-------------|
| `--config <path>` | Path to config directory (default: `.`) |
| `--env <name>` | Runtime environment (loads overlay) |

### Common Command Flags

| Command | Flag | Description |
|---------|------|-------------|
| `noda start` | `--server` | Start HTTP server only |
| `noda start` | `--workers` | Start worker runtime only |
| `noda start` | `--scheduler` | Start scheduler only |
| `noda start` | `--wasm` | Start Wasm runtimes only |
| `noda start` | `--all` | Start all runtimes (default) |
| `noda validate` | `--verbose` | Show detailed validation info |
| `noda test` | `--verbose` | Show execution traces for all tests |
| `noda test` | `--workflow <id>` | Run tests only for specified workflow |
| `noda generate openapi` | `--output <file>` | Output file path (default: stdout) |
| `noda migrate *` | `--service <name>` | Database service name (default: `db`) |
```

- [ ] **Step 2: Commit**

```bash
git add docs/01-getting-started/installation.md
git commit -m "docs: update installation guide with curl installer and Windows instructions"
```

---

### Task 5: Add `make install` Target

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add `install` target to Makefile**

Add the following after the existing `build-go` target (after line 17):

```makefile
install: build
	@if [ -w /usr/local/bin ]; then \
		cp dist/noda /usr/local/bin/noda; \
	elif command -v sudo >/dev/null 2>&1; then \
		sudo cp dist/noda /usr/local/bin/noda; \
	else \
		mkdir -p $(HOME)/.local/bin; \
		cp dist/noda $(HOME)/.local/bin/noda; \
		echo "Installed to $(HOME)/.local/bin/noda"; \
	fi
```

Also add `install` to the `.PHONY` list on line 1.

- [ ] **Step 2: Test `make install`**

Run: `make install`

Expected: Builds the editor and Go binary, then copies `dist/noda` to `/usr/local/bin/noda`. Verify with `which noda && noda version`.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "build: add make install target for local dev builds"
```

---

### Task 6: End-to-End Validation

This task validates the full pipeline. It requires pushing a tag.

- [ ] **Step 1: Create and push a test tag**

```bash
git tag v0.0.1-rc.1
git push origin v0.0.1-rc.1
```

- [ ] **Step 2: Monitor the release workflow**

Open https://github.com/chimpanze/noda/actions and watch the "Release" workflow. Verify:
- All three build jobs (linux, darwin, windows) complete successfully
- The merge job creates a GitHub Release at https://github.com/chimpanze/noda/releases/tag/v0.0.1-rc.1
- The release contains 6 archives + `checksums.txt`
- The release body includes the install instructions header

- [ ] **Step 3: Test the install script against the real release**

Run on macOS or Linux:
```bash
curl -fsSL https://raw.githubusercontent.com/chimpanze/noda/main/install.sh | VERSION=v0.0.1-rc.1 sh
```

Expected:
```
Installing noda v0.0.1-rc.1 (darwin/arm64)...
Downloading https://github.com/chimpanze/noda/releases/download/v0.0.1-rc.1/noda_0.0.1-rc.1_darwin_arm64.tar.gz...
Checksum verified.
Installed to /usr/local/bin/noda
noda 0.0.1-rc.1
...
noda installed successfully.
```

- [ ] **Step 4: Clean up test release (optional)**

If you want to remove the test release:
```bash
gh release delete v0.0.1-rc.1 --yes
git push origin --delete v0.0.1-rc.1
git tag -d v0.0.1-rc.1
```

- [ ] **Step 5: Fix any issues found, commit, and re-test if needed**

Iterate on any failures from the CI run or install script test. Each fix gets its own commit.
