#!/usr/bin/env bash
# Unified build dispatcher for WhatsApp Desk.
# Usage: ./build.sh [mac|linux|windows|all|check] [version]
# Delegates to the per-OS scripts so packaging logic stays in one place each.
# "check" compiles + vets all three OS targets without packaging anything,
# so Windows/Linux-only breakage is caught on this machine (no CI needed).
set -e

VERSION="${2:-1.5.9}"  # single source of truth, injected via -X main.appVersion
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

check_all() {
    echo "=== gofmt (excluding vendor) ==="
    UNFMT=$(gofmt -l . 2>/dev/null | grep -v "^vendor/" || true)
    if [ -n "${UNFMT}" ]; then
        echo "Unformatted files:"; echo "${UNFMT}"
        return 1
    fi
    echo "gofmt clean."

    echo "=== macOS compile ==="
    GOTOOLCHAIN=local go build -o /dev/null . || return 1

    echo "=== Windows cross-compile + vet ==="
    GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /dev/null . || return 1
    GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet . || return 1

    echo "=== Linux compile + vet (docker) ==="
    if ! command -v docker >/dev/null 2>&1; then
        echo "docker not found, skipping linux check."
        return 0
    fi
    BUILDER_CTX="${HOME}/.cache/wadesk-builder"
    mkdir -p "${BUILDER_CTX}"
    if [ ! -f "${BUILDER_CTX}/Dockerfile" ]; then
        cat > "${BUILDER_CTX}/Dockerfile" <<'DOCKER_EOF'
FROM golang:1.26-bookworm
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    libgtk-3-dev libwebkit2gtk-4.0-dev dpkg-dev zip \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /src
DOCKER_EOF
    fi
    if ! docker image inspect wadesk-linux-builder >/dev/null 2>&1; then
        echo "Building local builder image (one-time, cached afterwards)..."
        docker build --platform linux/amd64 -t wadesk-linux-builder "${BUILDER_CTX}" || return 1
    fi
    docker run --platform linux/amd64 --rm \
        -v "$PWD":/src -w /src \
        -v wadesk-gocache:/tmp/gocache -e GOCACHE=/tmp/gocache \
        wadesk-linux-builder \
        bash -c "go build -o /tmp/wa-check . && go vet ." || return 1

    echo "All checks passed."
}

case "${TARGET}" in
    mac)      build_mac ;;
    linux)    build_linux ;;
    windows)  build_windows ;;
    check)    check_all ;;
    all)
        case "$(uname -s)" in
            Darwin) build_mac ;;
            Linux)  build_linux ;;
            MINGW*|MSYS*) build_windows ;;
            *) echo "Unknown host $(uname -s); run with an explicit target."; exit 1 ;;
        esac
        ;;
    *) echo "Usage: $0 [mac|linux|windows|all|check] [version]"; exit 1 ;;
esac

echo "Done! (version ${VERSION})"
