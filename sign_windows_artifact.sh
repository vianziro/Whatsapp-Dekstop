#!/usr/bin/env bash
# Opt-in Authenticode signing for release artifacts. The certificate is
# supplied by the release operator/CI as a protected PFX file; it is never
# checked into the repository.
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "Usage: $0 <artifact.exe>" >&2
  exit 2
fi

ARTIFACT="$1"
if [ -z "${WA_WINDOWS_SIGN_CERT:-}" ]; then
  echo "Authenticode signing skipped (WA_WINDOWS_SIGN_CERT is not configured)."
  exit 0
fi
if [ ! -f "$WA_WINDOWS_SIGN_CERT" ]; then
  echo "ERROR: signing certificate not found: $WA_WINDOWS_SIGN_CERT" >&2
  exit 1
fi
if [ -z "${WA_WINDOWS_SIGN_PASSWORD:-}" ]; then
  echo "ERROR: WA_WINDOWS_SIGN_PASSWORD is required when signing is enabled." >&2
  exit 1
fi
if ! command -v signtool >/dev/null 2>&1; then
  echo "ERROR: signtool is required to sign Windows artifacts." >&2
  exit 1
fi
if [ ! -f "$ARTIFACT" ]; then
  echo "ERROR: artifact not found: $ARTIFACT" >&2
  exit 1
fi

TIMESTAMP_URL="${WA_WINDOWS_SIGN_TIMESTAMP_URL:-http://timestamp.digicert.com}"
signtool sign /fd SHA256 /td SHA256 /tr "$TIMESTAMP_URL" \
  /f "$WA_WINDOWS_SIGN_CERT" /p "$WA_WINDOWS_SIGN_PASSWORD" "$ARTIFACT"
signtool verify /pa /v "$ARTIFACT"
echo "Authenticode signature verified: $ARTIFACT"
