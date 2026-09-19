#!/usr/bin/env bash
set -e

APP_NAME="WhatsApp Desk"
VERSION="${1:-1.5.9.7}"  # single source of truth: injected via -X main.appVersion below
BINARY_NAME="WhatsApp"
BUNDLE_DIR="${APP_NAME}.app"
CONTENTS_DIR="${BUNDLE_DIR}/Contents"
MACOS_DIR="${CONTENTS_DIR}/MacOS"
RESOURCES_DIR="${CONTENTS_DIR}/Resources"

echo "Building universal binary for macOS (arm64 + x86_64)..."
rm -f "${BINARY_NAME}_mac" "${BINARY_NAME}_arm64" "${BINARY_NAME}_amd64"

# cgo does not invalidate its object cache when a vendored C/C++ *header*
# changes: Go hashes the package's .go/.c/.cc sources, not every transitively
# included .h. Editing libs/webview/include/webview.h therefore kept reusing a
# stale object file, and the shipped app silently ran without the per-account
# WKWebsiteDataStore code -- two accounts shared one WhatsApp session. Hash the
# native sources and purge the cache whenever they change so a header edit can
# never be ignored again.
NATIVE_SOURCES=(
    "vendor/github.com/webview/webview_go/webview.cc"
    "vendor/github.com/webview/webview_go/glue.c"
    "vendor/github.com/webview/webview_go/libs/webview/include/webview.h"
)
NATIVE_HASH="$(cat "${NATIVE_SOURCES[@]}" | shasum -a 256 | awk '{print $1}')"
STAMP_FILE=".build-native.hash"
if [ ! -f "${STAMP_FILE}" ] || [ "$(cat "${STAMP_FILE}")" != "${NATIVE_HASH}" ]; then
    echo "Native webview sources changed - clearing stale Go build cache..."
    go clean -cache
    printf '%s' "${NATIVE_HASH}" > "${STAMP_FILE}"
fi

LDFLAGS="-s -w -buildid= -X main.appVersion=${VERSION}"
LIPO="/usr/bin/lipo"
if [ -x "$LIPO" ] && \
   CGO_ENABLED=1 GOARCH=arm64 CGO_CFLAGS="-arch arm64" CGO_LDFLAGS="-arch arm64" GOTOOLCHAIN=local go build -ldflags="${LDFLAGS}" -trimpath -o "${BINARY_NAME}_arm64" . && \
   CGO_ENABLED=1 GOARCH=amd64 CGO_CFLAGS="-arch x86_64" CGO_LDFLAGS="-arch x86_64" GOTOOLCHAIN=local go build -ldflags="${LDFLAGS}" -trimpath -o "${BINARY_NAME}_amd64" .; then
    "$LIPO" -create -output "${BINARY_NAME}_mac" "${BINARY_NAME}_arm64" "${BINARY_NAME}_amd64"
    rm -f "${BINARY_NAME}_arm64" "${BINARY_NAME}_amd64"
    echo "Successfully built universal binary (Apple Silicon + Intel)"
else
    echo "Falling back to native host architecture build..."
    GOTOOLCHAIN=local go build -ldflags="${LDFLAGS}" -trimpath -o "${BINARY_NAME}_mac" .
fi

# Refuse to package a binary that lacks the profile-selection code. This is the
# exact failure that shipped a broken multi-account build, so it is now a hard
# build-time gate rather than something only noticed at runtime.
if ! grep -aq "wa-desk-profile" "${BINARY_NAME}_mac"; then
    echo "ERROR: ${BINARY_NAME}_mac is missing the WKWebsiteDataStore profile code." >&2
    echo "       Account isolation would silently fail. Aborting." >&2
    rm -f "${BINARY_NAME}_mac"
    exit 1
fi
echo "Verified: per-account profile code is present in the binary."

echo "Packaging ${BUNDLE_DIR}..."
rm -rf "${BUNDLE_DIR}"
mkdir -p "${MACOS_DIR}" "${RESOURCES_DIR}"

if [ -f "AppIcon.icns" ]; then
    cp "AppIcon.icns" "${RESOURCES_DIR}/AppIcon.icns"
fi

cp "${BINARY_NAME}_mac" "${MACOS_DIR}/${BINARY_NAME}"
chmod +x "${MACOS_DIR}/${BINARY_NAME}"

cat << EOF > "${CONTENTS_DIR}/Info.plist"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>WhatsApp</string>
    <key>CFBundleIdentifier</key>
    <string>com.whatsapp.desk</string>
    <key>CFBundleName</key>
    <string>WhatsApp Desk</string>
    <key>CFBundleDisplayName</key>
    <string>WhatsApp Desk</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>${VERSION}</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>NSRequiresAquaSystemAppearance</key>
    <false/>
    <key>NSCameraUsageDescription</key>
    <string>WhatsApp requires camera access for video calls.</string>
    <key>NSMicrophoneUsageDescription</key>
    <string>WhatsApp requires microphone access for voice and video calls.</string>
</dict>
</plist>
EOF

echo "Code signing ${BUNDLE_DIR} (ad-hoc, hardened runtime)..."
xattr -cr "${BUNDLE_DIR}" 2>/dev/null || true
if command -v codesign >/dev/null 2>&1; then
    if [ -f "WhatsAppDesk.entitlements" ]; then
        codesign --force --deep -s - --options runtime --entitlements WhatsAppDesk.entitlements "${BUNDLE_DIR}"
    else
        codesign --force --deep -s - --options runtime "${BUNDLE_DIR}"
    fi
    echo "Bundle successfully signed."
fi

if command -v hdiutil >/dev/null 2>&1; then
    echo "Creating DMG installer (WhatsApp-Desk-macOS-Universal.dmg)..."
    DMG_DIR="dmg_temp"
    rm -rf "${DMG_DIR}" "WhatsApp-Desk-macOS-Universal.dmg" "WhatsApp-macOS-Universal.dmg"
    mkdir -p "${DMG_DIR}"
    cp -R "${BUNDLE_DIR}" "${DMG_DIR}/"
    ln -s /Applications "${DMG_DIR}/Applications"
    if [ -f "dmg_background.png" ]; then
        mkdir -p "${DMG_DIR}/.background"
        cp "dmg_background.png" "${DMG_DIR}/.background/background.png"
    fi
    if hdiutil create -volname "WhatsApp Desk" -srcfolder "${DMG_DIR}" -ov -format UDZO "WhatsApp-Desk-macOS-Universal.dmg" 2>/dev/null; then
        cp "WhatsApp-Desk-macOS-Universal.dmg" "WhatsApp-macOS-Universal.dmg"
        echo "Successfully created DMG installer."
    else
        echo "Notice: DMG creation skipped in sandbox environment. ZIP bundle will be created."
    fi
    rm -rf "${DMG_DIR}"
fi

echo "Creating ZIP bundle (WhatsApp-Desk-macOS-Universal.zip)..."
rm -f "WhatsApp-Desk-macOS-Universal.zip" "WhatsApp-macOS-Universal.zip"
zip -r -y -q "WhatsApp-Desk-macOS-Universal.zip" "${BUNDLE_DIR}"
cp "WhatsApp-Desk-macOS-Universal.zip" "WhatsApp-macOS-Universal.zip"

echo "Done! Built ${BUNDLE_DIR} and ZIP bundle."
