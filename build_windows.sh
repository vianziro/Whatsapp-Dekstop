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

# Version SSOT: appVersion in updater.go. A bare call used to default to a
# stale hardcoded number, which stamped an old appVersion into the binary and
# made the in-app updater believe the app was outdated forever.
VERSION="${1:-$(sed -n 's/^var appVersion = "\(.*\)"$/\1/p' updater.go)}"
if [ -z "$VERSION" ]; then
  echo "ERROR: cannot read appVersion from updater.go; pass the version as \$1" >&2
  exit 1
fi
ARCH="${2:-amd64}"
case "$ARCH" in
  amd64) OUT="dist_win/WhatsAppDesk.exe"; MINGW_CC=x86_64-w64-mingw32-gcc ;;
  # Windows on ARM. Native build of the same source; the updater picks it
  # by this name (windowsArm64AssetName in updater.go).
  arm64) OUT="dist_win/WhatsAppDesk-arm64.exe"; MINGW_CC=aarch64-w64-mingw32-gcc ;;
  *) echo "ERROR: unsupported Windows arch: $ARCH (amd64 or arm64)" >&2; exit 2 ;;
esac
[ -f "rsrc_windows_${ARCH}.syso" ] || { echo "ERROR: rsrc_windows_${ARCH}.syso is missing; run make_windows_resources.sh" >&2; exit 1; }
mkdir -p dist_win

export GOOS=windows
export GOARCH="$ARCH"

if command -v "$MINGW_CC" >/dev/null 2>&1; then
  export CGO_ENABLED=1
  export CC="$MINGW_CC"
else
  export CGO_ENABLED=0
fi

go build -ldflags="-H windowsgui -X main.appVersion=${VERSION}" \
  -trimpath -o "$OUT" .

bash sign_windows_artifact.sh "$OUT"

echo "built: $OUT (CGO_ENABLED=$CGO_ENABLED)"
ls -la "$OUT"
file "$OUT"
