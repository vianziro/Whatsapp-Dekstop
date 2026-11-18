#!/usr/bin/env bash
# Regenerates the Windows resource objects (application icon + manifest) that
# the Go linker embeds into WhatsAppDesk.exe, one per architecture:
#
#   rsrc_windows_amd64.syso   x64 build
#   rsrc_windows_arm64.syso   Windows on ARM build
#
# Run it after changing icon.ico, app.manifest or resource.rc, and commit the
# .syso files. The x64 object used to be a copy from the original upstream
# project, so the .exe carried that project's icon instead of icon.ico.
#
# Needs llvm-windres (brew install llvm), which can target both architectures;
# GNU windres from mingw-w64 only writes x86 objects.
set -euo pipefail
cd "$(dirname "$0")"

WINDRES="${WINDRES:-}"
if [ -z "$WINDRES" ]; then
  for candidate in llvm-windres /opt/homebrew/opt/llvm/bin/llvm-windres /usr/local/opt/llvm/bin/llvm-windres; do
    if command -v "$candidate" >/dev/null 2>&1; then
      WINDRES="$candidate"
      break
    fi
  done
fi
if [ -z "$WINDRES" ]; then
  echo "ERROR: llvm-windres not found (brew install llvm, or set WINDRES=...)" >&2
  exit 1
fi

for pair in "amd64:x86_64-w64-mingw32" "arm64:aarch64-w64-mingw32"; do
  arch="${pair%%:*}"
  target="${pair#*:}"
  "$WINDRES" --target="$target" -O coff -i resource.rc -o "rsrc_windows_${arch}.syso"
  echo "wrote rsrc_windows_${arch}.syso ($(wc -c < "rsrc_windows_${arch}.syso" | tr -d ' ') bytes)"
done
