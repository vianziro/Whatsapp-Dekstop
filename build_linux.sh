#!/usr/bin/env bash
set -e

APP_NAME="whatsapp-desk"
DISPLAY_NAME="WhatsApp Desk"
VERSION="${1:-${WA_DESK_VERSION:-1.5.9.1}}"
OUTPUT_DIR="dist_linux"

# Derive the target architecture from the Go toolchain instead of hardcoding it.
# Hardcoding amd64 silently mislabelled arm64 builds: the .deb control file and
# the artifact names claimed amd64 while the binary inside was aarch64.
GOARCH_VALUE="$(go env GOARCH 2>/dev/null || true)"
case "${GOARCH_VALUE}" in
    amd64) DEB_ARCH="amd64"; BUNDLE_ARCH="x64" ;;
    arm64) DEB_ARCH="arm64"; BUNDLE_ARCH="arm64" ;;
    "")    DEB_ARCH="amd64"; BUNDLE_ARCH="x64" ;;
    *)     DEB_ARCH="${GOARCH_VALUE}"; BUNDLE_ARCH="${GOARCH_VALUE}" ;;
esac
echo "Target architecture: ${GOARCH_VALUE:-unknown} (deb: ${DEB_ARCH}, bundle: ${BUNDLE_ARCH})"

echo "=== Building ${DISPLAY_NAME} for Linux ==="

# Check prerequisites on Linux host/CI
if command -v pkg-config >/dev/null 2>&1; then
    if ! pkg-config --exists gtk+-3.0 webkit2gtk-4.0 && ! pkg-config --exists webkit2gtk-4.1; then
        echo "Warning: GTK3 and WebKit2GTK-4.0 development headers are required to build on Linux."
        echo "Debian/Ubuntu: sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.0-dev"
        echo "Fedora/RHEL:   sudo dnf install -y gtk3-devel webkit2gtk4.0-devel"
    fi
fi

rm -rf "${OUTPUT_DIR}" "${APP_NAME}"
mkdir -p "${OUTPUT_DIR}"

echo "Compiling Linux binary..."
go build -ldflags="-s -w -buildid= -X main.appVersion=${VERSION}" -trimpath -o "${OUTPUT_DIR}/${APP_NAME}" .

# Copy assets
cp icon.png "${OUTPUT_DIR}/"

# Generate .desktop launcher
cat << EOF > "${OUTPUT_DIR}/${APP_NAME}.desktop"
[Desktop Entry]
Type=Application
Version=1.0
Name=${DISPLAY_NAME}
Comment=Lightweight WhatsApp Desktop Client
Exec=${APP_NAME}
Icon=${APP_NAME}
Terminal=false
Categories=Network;InstantMessaging;Chat;
StartupWMClass=${APP_NAME}
Keywords=WhatsApp;Chat;Messenger;
EOF

# Create tar.gz release bundle
echo "Creating release tarball (WhatsApp-Desk-Linux-${BUNDLE_ARCH}.tar.gz)..."
TAR_BUNDLE="WhatsApp-Desk-Linux-${BUNDLE_ARCH}.tar.gz"
rm -f "${TAR_BUNDLE}"
tar -czf "${TAR_BUNDLE}" -C "${OUTPUT_DIR}" .

# Build .deb package if dpkg-deb is available
if command -v dpkg-deb >/dev/null 2>&1; then
    echo "Building Debian package (.deb)..."
    DEB_DIR="deb_temp"
    rm -rf "${DEB_DIR}"
    mkdir -p "${DEB_DIR}/DEBIAN"
    mkdir -p "${DEB_DIR}/usr/bin"
    mkdir -p "${DEB_DIR}/usr/share/applications"
    mkdir -p "${DEB_DIR}/usr/share/icons/hicolor/512x512/apps"

    cp "${OUTPUT_DIR}/${APP_NAME}" "${DEB_DIR}/usr/bin/"
    chmod 755 "${DEB_DIR}/usr/bin/${APP_NAME}"

    cp "${OUTPUT_DIR}/${APP_NAME}.desktop" "${DEB_DIR}/usr/share/applications/"
    chmod 644 "${DEB_DIR}/usr/share/applications/${APP_NAME}.desktop"

    cp icon.png "${DEB_DIR}/usr/share/icons/hicolor/512x512/apps/${APP_NAME}.png"
    chmod 644 "${DEB_DIR}/usr/share/icons/hicolor/512x512/apps/${APP_NAME}.png"

    cat << EOF > "${DEB_DIR}/DEBIAN/control"
Package: ${APP_NAME}
Version: ${VERSION}
Architecture: ${DEB_ARCH}
Maintainer: Community <contact@github.com>
Installed-Size: 15000
Depends: libgtk-3-0, libwebkit2gtk-4.0-37 | libwebkit2gtk-4.1-0
Section: net
Priority: optional
Description: Lightweight WhatsApp Desktop Client
 Independent, fast, and privacy-focused WhatsApp desktop application
 built with Go and native WebKit.
EOF

    dpkg-deb --build "${DEB_DIR}" "whatsapp-desk_${VERSION}_${DEB_ARCH}.deb"
    cp "whatsapp-desk_${VERSION}_${DEB_ARCH}.deb" "WhatsApp-Desk-Linux-${DEB_ARCH}.deb"
    rm -rf "${DEB_DIR}"
    echo "Created: whatsapp-desk_${VERSION}_${DEB_ARCH}.deb and WhatsApp-Desk-Linux-${DEB_ARCH}.deb"
fi

echo "Done! Linux artifacts ready in ${OUTPUT_DIR} and ${TAR_BUNDLE}."
