#!/bin/bash
# Builds the Windows GUI binary from the current tree with the version injected
# at link time.
#
# Kept as a script (rather than an inline command) because the interactive shell
# on this host loads a mise hook that mangles compound one-liners.
#
# ALIGNMENT WITH CI: this mirrors build.sh's Windows target on purpose, so a
# locally produced release matches what the GitHub workflow publishes.
#   * CGO is enabled when a MinGW cross-compiler is present, and disabled
#     otherwise (the go-webview2 binding is pure Go, so both work — but the
#     binaries differ, and a release should not depend on which host built it).
#   * The binary is deliberately NOT stripped: an unsigned, aggressively
#     stripped GUI executable is more likely to be flagged heuristically by
#     endpoint protection, so symbol metadata is kept.
set -euo pipefail
cd "$(dirname "$0")"

VERSION="${1:-1.5.9.7}"
ARCH="${2:-amd64}"
OUT="dist_win/WhatsAppDesk.exe"
mkdir -p dist_win

export GOOS=windows
export GOARCH="$ARCH"

if command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
  export CGO_ENABLED=1
  export CC=x86_64-w64-mingw32-gcc
else
  export CGO_ENABLED=0
fi

go build -ldflags="-H windowsgui -X main.appVersion=${VERSION}" \
  -trimpath -o "$OUT" .

echo "built: $OUT (CGO_ENABLED=$CGO_ENABLED)"
ls -la "$OUT"
file "$OUT"
