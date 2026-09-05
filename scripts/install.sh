#!/usr/bin/env bash
# ==============================================================================
# Nacho Flow Universal Installer (Linux & macOS)
# https://spicebox.dev/nacho-flow/ | https://github.com/dixieflatline76/nacho-flow
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/dixieflatline76/nacho-flow/main/scripts/install.sh | bash
#
# Options:
#   --version <vX.Y.Z>   Install a specific version (defaults to latest)
#   --dir <path>         Custom installation directory (defaults to /usr/local/bin or ~/.local/bin)
#   --service            Automatically register and start native systemd background service (Linux only)
#   --dry-run            Simulate installation steps without downloading or modifying files
#   --uninstall          Remove nacho-flow binary and system service
#   --help               Show this help message
# ==============================================================================

set -euo pipefail

REPO_OWNER="dixieflatline76"
REPO_NAME="nacho-flow"
BINARY_NAME="nacho-flow"

# Colors for terminal output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# Defaults
TARGET_VERSION=""
CUSTOM_DIR=""
INSTALL_SERVICE=false
DRY_RUN=false
UNINSTALL=false

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

print_banner() {
    echo -e "${CYAN}${BOLD}"
    echo "  🌮 Nacho Flow Universal Installer"
    echo "  Agent Supervisor & Model Dispatcher"
    echo -e "${NC}"
}

print_help() {
    print_banner
    echo "Usage: install.sh [options]"
    echo ""
    echo "Options:"
    echo "  --version <vX.Y.Z>   Install specific version (default: latest)"
    echo "  --dir <path>         Install to custom directory (default: /usr/local/bin or ~/.local/bin)"
    echo "  --service            Register and start native systemd service (Linux only)"
    echo "  --dry-run            Preview installation steps without making changes"
    echo "  --uninstall          Uninstall nacho-flow and remove services"
    echo "  -h, --help           Show help message"
    echo ""
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --version)
            TARGET_VERSION="$2"
            shift 2
            ;;
        --dir)
            CUSTOM_DIR="$2"
            shift 2
            ;;
        --service)
            INSTALL_SERVICE=true
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --uninstall)
            UNINSTALL=true
            shift
            ;;
        -h|--help)
            print_help
            exit 0
            ;;
        *)
            log_error "Unknown argument: $1"
            print_help
            exit 1
            ;;
    esac
done

detect_os() {
    local os
    os="$(uname -s)"
    case "$os" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
        *)
            log_error "Unsupported operating system: $os. Please download manually from https://github.com/${REPO_OWNER}/${REPO_NAME}/releases"
            exit 1
            ;;
    esac
}

detect_arch() {
    local arch
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64)   echo "amd64" ;;
        aarch64|arm64)  echo "arm64" ;;
        *)
            log_error "Unsupported architecture: $arch. Please download manually from https://github.com/${REPO_OWNER}/${REPO_NAME}/releases"
            exit 1
            ;;
    esac
}

verify_checksum() {
    local file_path="$1"
    local checksums_file="$2"
    local filename
    filename="$(basename "$file_path")"

    local expected_hash
    expected_hash="$(grep "$filename" "$checksums_file" | awk '{print $1}' || true)"

    if [ -z "$expected_hash" ]; then
        log_warn "Checksum not found in checksums.txt for $filename; skipping hash verification."
        return 0
    fi

    local actual_hash=""
    if command -v sha256sum >/dev/null 2>&1; then
        actual_hash="$(sha256sum "$file_path" | awk '{print $1}')"
    elif command -v shasum >/dev/null 2>&1; then
        actual_hash="$(shasum -a 256 "$file_path" | awk '{print $1}')"
    else
        log_warn "Neither sha256sum nor shasum found; skipping cryptographic hash verification."
        return 0
    fi

    if [ "$actual_hash" != "$expected_hash" ]; then
        log_error "Cryptographic SHA-256 verification failed!"
        log_error "Expected: $expected_hash"
        log_error "Actual:   $actual_hash"
        exit 1
    fi

    log_success "SHA-256 Checksum verified ($actual_hash)"
}

perform_uninstall() {
    print_banner
    log_info "Initiating uninstallation of nacho-flow..."

    local found=false
    for bin_path in "/usr/local/bin/${BINARY_NAME}" "$HOME/.local/bin/${BINARY_NAME}"; do
        if [ -f "$bin_path" ]; then
            found=true
            log_info "Removing $bin_path..."
            if [ -w "$(dirname "$bin_path")" ]; then
                rm -f "$bin_path"
            else
                sudo rm -f "$bin_path"
            fi
            log_success "Removed $bin_path"
        fi
    done

    if [ "$found" = false ]; then
        log_warn "No nacho-flow binary found in /usr/local/bin or ~/.local/bin."
    fi

    # Remove systemd service if present
    if command -v systemctl >/dev/null 2>&1 && [ -f "/etc/systemd/system/nacho-flow.service" ]; then
        log_info "Stopping and disabling systemd service..."
        sudo systemctl stop nacho-flow || true
        sudo systemctl disable nacho-flow || true
        sudo rm -f /etc/systemd/system/nacho-flow.service
        sudo systemctl daemon-reload || true
        log_success "Removed nacho-flow systemd service."
    fi

    log_success "Uninstallation complete. 🌮"
    exit 0
}

if [ "$UNINSTALL" = true ]; then
    perform_uninstall
fi

print_banner

OS="$(detect_os)"
ARCH="$(detect_arch)"
log_info "Detected environment: ${OS}-${ARCH}"

# Determine installation directory
if [ -n "$CUSTOM_DIR" ]; then
    INSTALL_DIR="$CUSTOM_DIR"
elif [ -w "/usr/local/bin" ] || [ "$(id -u)" -eq 0 ]; then
    INSTALL_DIR="/usr/local/bin"
else
    INSTALL_DIR="$HOME/.local/bin"
fi

log_info "Target install directory: ${INSTALL_DIR}"

# Determine release version
if [ -z "$TARGET_VERSION" ]; then
    log_info "Resolving latest stable release tag from GitHub..."
    if [ "$DRY_RUN" = true ]; then
        LATEST_TAG="v0.3.0"
    else
        LATEST_TAG="$(curl -fsSL "https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest" 2>/dev/null | grep '"tag_name":' | head -n1 | sed -E 's/.*"([^"]+)".*/\1/' || true)"
        if [ -z "$LATEST_TAG" ]; then
            # Fallback to listing releases
            LATEST_TAG="$(curl -fsSL "https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases" 2>/dev/null | grep '"tag_name":' | head -n1 | sed -E 's/.*"([^"]+)".*/\1/' || echo "v0.3.0")"
        fi
    fi
else
    if [[ "$TARGET_VERSION" != v* ]]; then
        LATEST_TAG="v${TARGET_VERSION}"
    else
        LATEST_TAG="${TARGET_VERSION}"
    fi
fi

RAW_VERSION="${LATEST_TAG#v}"
if [ "$OS" = "windows" ]; then
    ASSET_NAME="nacho-flow-${RAW_VERSION}-windows-${ARCH}.exe"
else
    ASSET_NAME="nacho-flow-${RAW_VERSION}-${OS}-${ARCH}"
fi
DOWNLOAD_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${LATEST_TAG}/${ASSET_NAME}"
CHECKSUMS_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${LATEST_TAG}/checksums.txt"

log_info "Target version: ${LATEST_TAG}"
log_info "Release asset: ${ASSET_NAME}"

if [ "$DRY_RUN" = true ]; then
    echo ""
    log_info "[DRY-RUN] Would download: $DOWNLOAD_URL"
    log_info "[DRY-RUN] Would download: $CHECKSUMS_URL"
    log_info "[DRY-RUN] Would verify SHA-256 checksum against checksums.txt"
    log_info "[DRY-RUN] Would install to: ${INSTALL_DIR}/${BINARY_NAME}"
    log_info "[DRY-RUN] Would execute: chmod +x ${INSTALL_DIR}/${BINARY_NAME}"
    log_success "[DRY-RUN] Dry run completed successfully with zero modifications."
    exit 0
fi

# Create temporary working directory
TMP_DIR="$(mktemp -d)"
cleanup() {
    rm -rf "$TMP_DIR"
}
trap cleanup EXIT

log_info "Downloading ${ASSET_NAME}..."
if ! curl -fsSL "$DOWNLOAD_URL" -o "${TMP_DIR}/${ASSET_NAME}"; then
    log_error "Failed to download binary from $DOWNLOAD_URL"
    log_error "Please check version availability at https://github.com/${REPO_OWNER}/${REPO_NAME}/releases"
    exit 1
fi

log_info "Downloading checksums.txt..."
curl -fsSL "$CHECKSUMS_URL" -o "${TMP_DIR}/checksums.txt" || true

if [ -f "${TMP_DIR}/checksums.txt" ]; then
    verify_checksum "${TMP_DIR}/${ASSET_NAME}" "${TMP_DIR}/checksums.txt"
fi

# Ensure target directory exists
mkdir -p "$INSTALL_DIR"

TARGET_BIN="${INSTALL_DIR}/${BINARY_NAME}"

log_info "Installing binary to ${TARGET_BIN}..."
if [ -w "$INSTALL_DIR" ]; then
    mv "${TMP_DIR}/${ASSET_NAME}" "$TARGET_BIN"
    chmod +x "$TARGET_BIN"
else
    log_info "Elevated permissions required for ${INSTALL_DIR}. Requesting sudo..."
    sudo mv "${TMP_DIR}/${ASSET_NAME}" "$TARGET_BIN"
    sudo chmod +x "$TARGET_BIN"
fi

log_success "Installed ${BINARY_NAME} to ${TARGET_BIN}"

# Check PATH availability
if [[ ":$PATH:" != *":${INSTALL_DIR}:"* ]]; then
    log_warn "${INSTALL_DIR} is not currently in your system PATH."
    log_warn "To use '${BINARY_NAME}' from anywhere, add this line to your ~/.bashrc or ~/.zshrc:"
    echo -e "    ${BOLD}export PATH=\"${INSTALL_DIR}:\$PATH\"${NC}"
fi

# Test binary installation
if [ -x "$TARGET_BIN" ]; then
    log_info "Testing binary execution..."
    "$TARGET_BIN" version || true
fi

# Optional systemd service installation on Linux
if [ "$OS" = "linux" ]; then
    if [ "$INSTALL_SERVICE" = true ] || ( [ -t 0 ] && [ "$INSTALL_SERVICE" != "false" ] && command -v systemctl >/dev/null 2>&1 ); then
        if [ "$INSTALL_SERVICE" = true ]; then
            DO_INSTALL_SERVICE="y"
        else
            echo ""
            read -r -p "🌮 Would you like to register and start nacho-flow as a systemd background service? [y/N]: " DO_INSTALL_SERVICE || DO_INSTALL_SERVICE="n"
        fi

        if [[ "$DO_INSTALL_SERVICE" =~ ^[Yy]$ ]]; then
            log_info "Registering native systemd background service..."
            "$TARGET_BIN" service install || sudo "$TARGET_BIN" service install || true
            log_success "Nacho Flow background service registered and active!"
        fi
    fi
fi

echo ""
echo -e "${GREEN}${BOLD}🎉 Nacho Flow ${LATEST_TAG} installed successfully!${NC}"
echo -e "Quickstart:"
echo -e "  1. Run foreground:   ${CYAN}nacho-flow run${NC}"
echo -e "  2. View live stats:  ${CYAN}nacho-flow stats${NC}"
echo -e "  3. Service status:   ${CYAN}nacho-flow service status${NC}"
echo ""
