#!/usr/bin/env bash
# Unified build dispatcher for WhatsApp Desk.
# Usage: ./build.sh [mac|linux|windows|all] [version]
# Delegates to the per-OS scripts so packaging logic stays in one place each.
set -e

VERSION="${2:-1.5.7}"  # single source of truth, injected via -X main.appVersion
TARGET="${1:-all}"

export WA_DESK_VERSION="${VERSION}"

build_mac() {
    echo "=== macOS (universal) ==="
    bash build_mac.sh "${VERSION}"
}

build_linux() {
    echo "=== Linux ==="
    bash build_linux.sh "${VERSION}"
}

build_windows() {
    echo "=== Windows (x64) ==="
    if [ "$(uname -s)" != "MINGW"* ] && [ "$(uname -s)" != "MSYS"* ] && [ "${GOOS:-}" != "windows" ]; then
        echo "Note: Windows builds need a Windows host (or mingw-w64 + WebView2 SDK)."
    fi
    if ! command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1 && [ "$(uname -s)" != "MINGW"* ]; then
        echo "Warning: mingw-w64 cross compiler not found; attempting native toolchain."
    fi
    rm -f WhatsAppDesk.exe
    GOOS=windows GOARCH=amd64 CGO_ENABLED=1 \
        go build -ldflags="-s -w -buildid= -H windowsgui -X main.appVersion=${VERSION}" \
        -trimpath -o WhatsAppDesk.exe .
    rm -f WhatsApp-Desk-Windows-x64.zip
    zip -q WhatsApp-Desk-Windows-x64.zip WhatsAppDesk.exe
    echo "Created: WhatsAppDesk.exe and WhatsApp-Desk-Windows-x64.zip"
}

case "${TARGET}" in
    mac)      build_mac ;;
    linux)    build_linux ;;
    windows)  build_windows ;;
    all)
        case "$(uname -s)" in
            Darwin) build_mac ;;
            Linux)  build_linux ;;
            MINGW*|MSYS*) build_windows ;;
            *) echo "Unknown host $(uname -s); run with an explicit target."; exit 1 ;;
        esac
        ;;
    *) echo "Usage: $0 [mac|linux|windows|all] [version]"; exit 1 ;;
esac

echo "Done! (version ${VERSION})"
