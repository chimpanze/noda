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
