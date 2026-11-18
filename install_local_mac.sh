#!/usr/bin/env bash
# Build WhatsApp Desk from this folder and replace the copy in /Applications.
#
#   bash install_local_mac.sh
#
# Steps: tests -> stable signing identity (once) -> build -> quit the running
# app -> keep the old app as a backup -> install the new one -> launch it.
# Nothing is deleted: the previous app is moved to ~/WhatsApp-Desk-backups/.
set -euo pipefail
SCRIPT="$(cd "$(dirname "$0")" && pwd)/$(basename "$0")"
cd "$(dirname "$SCRIPT")"

APP_NAME="WhatsApp Desk"
TARGET="/Applications/${APP_NAME}.app"
BACKUP_DIR="$HOME/WhatsApp-Desk-backups"

say() { printf '\n\033[1;32m==> %s\033[0m\n' "$*"; }
die() { printf '\n\033[1;31mERROR: %s\033[0m\n' "$*" >&2; exit 1; }

[ "$(uname -s)" = "Darwin" ] || die "Run this on macOS."

# Go: use the system one, or Nix if that is where Go lives on this Mac.
if ! command -v go >/dev/null 2>&1; then
    if command -v nix >/dev/null 2>&1; then
        say "Go not on PATH; re-running inside 'nix shell nixpkgs#go'"
        exec nix --extra-experimental-features 'nix-command flakes' shell nixpkgs#go -c bash "$SCRIPT" "$@"
    fi
    die "Go is not installed (brew install go, or install Nix)."
fi
xcode-select -p >/dev/null 2>&1 || die "Xcode Command Line Tools are missing: run 'xcode-select --install'."

say "Go: $(go version)"
# go.mod asks for Go 1.26; let the toolchain fetch it if the local one is older.
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"

say "Running tests"
go test -count=1 ./... || die "Tests failed - nothing was installed. Paste the output above to Claude."

say "Stable signing identity (keeps the keychain 'Always Allow' across builds)"
bash make_mac_signing_identity.sh

say "Building ${APP_NAME} (this takes a few minutes)"
bash build_mac.sh || die "Build failed - nothing was installed. Paste the output above to Claude."
[ -d "${APP_NAME}.app" ] || die "Build finished but '${APP_NAME}.app' is missing."
codesign --verify --deep --strict "${APP_NAME}.app" || die "The new app's signature does not verify."

say "Quitting the running app (if any)"
osascript -e "tell application \"${APP_NAME}\" to quit" >/dev/null 2>&1 || true
for _ in $(seq 1 30); do
    pgrep -f "${TARGET}/Contents/MacOS/" >/dev/null 2>&1 || break
    sleep 1
done
pgrep -f "${TARGET}/Contents/MacOS/" >/dev/null 2>&1 && die "${APP_NAME} is still running; quit it and run this again."

if [ -d "${TARGET}" ]; then
    mkdir -p "${BACKUP_DIR}"
    BACKUP="${BACKUP_DIR}/${APP_NAME} $(date +%Y%m%d-%H%M%S).app"
    say "Keeping the old app as: ${BACKUP}"
    mv "${TARGET}" "${BACKUP}" || die "Could not move the old app (permission?). Nothing changed."
fi

say "Installing to ${TARGET}"
if ! ditto "${APP_NAME}.app" "${TARGET}"; then
    [ -n "${BACKUP:-}" ] && mv "${BACKUP}" "${TARGET}"
    die "Install failed; the previous app was put back."
fi
xattr -dr com.apple.quarantine "${TARGET}" 2>/dev/null || true

NEW_VERSION="$(defaults read "${TARGET}/Contents/Info" CFBundleShortVersionString 2>/dev/null || echo '?')"
say "Installed ${APP_NAME} ${NEW_VERSION}. Launching..."
open "${TARGET}"

cat <<'EOF'

Next:
  * If macOS asks about "WhatsApp Desk WebCrypto Master Key": enter your Mac
    login password and click "Always Allow" (never "Deny"). Future builds made
    with this script will not ask again.
  * Your chats and both accounts stay as they were; the session data lives in
    ~/Library, not inside the app.
  * Old app backup: ~/WhatsApp-Desk-backups/ (delete it once you're happy).
EOF
