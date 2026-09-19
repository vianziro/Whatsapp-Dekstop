#!/bin/bash
# Generates SHA256SUMS for a release and uploads it, so the file always
# describes exactly the artifacts that are actually published.
#
# The GitHub workflow builds this in CI, but releases are also cut by hand on
# this machine, and the README tells users to verify their download against
# this file — a release without it is a broken promise, and a file listing
# artifacts that are not published is worse still.
#
#   bash make_checksums.sh v1.5.9.3          # local check + upload
#   bash make_checksums.sh v1.5.9.3 --dry-run
#
# The artifact list is read from the GitHub release, so it cannot drift from
# what users can actually download. Format is GNU coreutils
# ("<digest>  <name>", two spaces) so `sha256sum -c SHA256SUMS` and
# `shasum -a 256 -c SHA256SUMS` both work.
set -euo pipefail
cd "$(dirname "$0")"

REPO="vianziro/Whatsapp-Dekstop"
TAG="${1:-}"
DRY_RUN="${2:-}"

if [ -z "$TAG" ]; then
  echo "usage: bash make_checksums.sh <tag> [--dry-run]" >&2
  echo "example: bash make_checksums.sh v1.5.9.3" >&2
  exit 2
fi

# Published assets, minus the checksum file itself. Read line by line rather
# than via mapfile: macOS still ships bash 3.2, which has no mapfile builtin.
ASSET_LIST="$(gh release view "$TAG" --repo "$REPO" --json assets \
  --jq '.assets[].name' | grep -v '^SHA256SUMS$' | sort)"

if [ -z "$ASSET_LIST" ]; then
  echo "release $TAG has no downloadable assets" >&2
  exit 1
fi

rm -f SHA256SUMS
found=0
missing=""
while IFS= read -r name; do
  [ -n "$name" ] || continue
  if [ -f "$name" ]; then
    shasum -a 256 "$name" >> SHA256SUMS
    found=$((found + 1))
  else
    missing="$missing  $name
"
  fi
done <<EOF
$ASSET_LIST
EOF

if [ -n "$missing" ]; then
  echo "error: published but not present locally — build them before checksumming:" >&2
  printf '%s' "$missing" >&2
  exit 1
fi

echo "wrote SHA256SUMS with $found artifact(s) for $TAG:"
cat SHA256SUMS

if [ "$DRY_RUN" = "--dry-run" ]; then
  echo "(dry run: not uploading)"
  exit 0
fi

gh release upload "$TAG" SHA256SUMS --clobber --repo "$REPO"
echo "uploaded SHA256SUMS to $TAG"
