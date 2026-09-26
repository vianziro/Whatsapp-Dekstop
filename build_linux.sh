#!/usr/bin/env bash
set -e

APP_NAME="whatsapp-desk"
DISPLAY_NAME="WhatsApp Desk"
VERSION="${1:-${WA_DESK_VERSION:-1.5.9.3}}"
OUTPUT_DIR="dist_linux"

# WebKitGTK variant selector (opt-in, default unchanged).
#   WA_DESK_WEBKIT=4.0  -> historical behavior: link against webkit2gtk-4.0
#                          (Ubuntu 22.04 Jammy and older Debian).
#   WA_DESK_WEBKIT=4.1  -> link against webkit2gtk-4.1 / libsoup-3.0
#                          (Ubuntu 24.04 Noble, Linux Mint 22.x, current Fedora).
# The default stays 4.0 so existing distros keep working; Noble+ users should
# build with WA_DESK_WEBKIT=4.1. Related to issues #8 and #9.
WEBKIT_VARIANT="${WA_DESK_WEBKIT:-4.0}"
case "${WEBKIT_VARIANT}" in
    4.0|4.1) ;;
    *) echo "error: WA_DESK_WEBKIT must be 4.0 or 4.1, got '${WEBKIT_VARIANT}'" >&2; exit 2 ;;
esac

# Artifacts of the 4.1 variant carry a suffix so they can never overwrite or
# be confused with the historical 4.0 artifacts. The DEB Depends of the 4.1
# variant names only the library the binary is actually linked against.
ARTIFACT_SUFFIX=""
DEB_DEPENDS="libgtk-3-0, libwebkit2gtk-4.0-37 | libwebkit2gtk-4.1-0"
if [ "${WEBKIT_VARIANT}" = "4.1" ]; then
    ARTIFACT_SUFFIX="-webkit4.1"
    DEB_DEPENDS="libgtk-3-0, libwebkit2gtk-4.1-0"
fi

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

echo "=== Building ${DISPLAY_NAME} for Linux (WebKitGTK ${WEBKIT_VARIANT}) ==="

# Check prerequisites on Linux host/CI
if command -v pkg-config >/dev/null 2>&1; then
    if [ "${WEBKIT_VARIANT}" = "4.1" ]; then
        if ! pkg-config --exists gtk+-3.0 webkit2gtk-4.1; then
            echo "Warning: GTK3 and WebKit2GTK-4.1 development headers are required for WA_DESK_WEBKIT=4.1."
            echo "Debian/Ubuntu (24.04+): sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.1-dev"
            echo "Fedora/RHEL:            sudo dnf install -y gtk3-devel webkit2gtk4.1-devel"
        fi
        # The vendored webview cgo requests the historical 4.0 pkg-config name.
        # For the 4.1 variant, always alias it to the real 4.1 .pc file so the
        # linker emits DT_NEEDED libwebkit2gtk-4.1.so.0 (verified via ldd).
        # The vendor tree itself is left untouched; the alias lives in a temp
        # dir that is removed on exit.
        if pkg-config --exists webkit2gtk-4.1; then
            WEBKIT_PC_DIR="$(pkg-config --variable=pcfiledir webkit2gtk-4.1)"
            WEBKIT_PC_ALIAS_DIR_41="$(mktemp -d)"
            ln -s "${WEBKIT_PC_DIR}/webkit2gtk-4.1.pc" "${WEBKIT_PC_ALIAS_DIR_41}/webkit2gtk-4.0.pc"
            export PKG_CONFIG_PATH="${WEBKIT_PC_ALIAS_DIR_41}${PKG_CONFIG_PATH:+:${PKG_CONFIG_PATH}}"
            trap 'rm -rf "${WEBKIT_PC_ALIAS_DIR_41:-/tmp/wa-desk-noop-41}" "${WEBKIT_PC_ALIAS_DIR:-/tmp/wa-desk-noop-40}"' EXIT
            echo "WebKitGTK 4.1 variant: aliasing webkit2gtk-4.0.pc -> webkit2gtk-4.1.pc for this build only."
        fi
    else
        if ! pkg-config --exists gtk+-3.0 webkit2gtk-4.0 && ! pkg-config --exists webkit2gtk-4.1; then
            echo "Warning: GTK3 and WebKit2GTK development headers are required to build on Linux."
            echo "Debian/Ubuntu (22.04): sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.0-dev"
            echo "Debian/Ubuntu (24.04+): sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.1-dev (then build with WA_DESK_WEBKIT=4.1)"
            echo "Fedora/RHEL:   sudo dnf install -y gtk3-devel webkit2gtk4.1-devel"
        fi
        # Legacy auto-alias, preserved verbatim for the default 4.0 path:
        # hosts with only 4.1 (e.g. Fedora) can still compile, but the
        # resulting binary is 4.1-linked while the generic artifact name and
        # the transitional Depends suggest 4.0. Prefer WA_DESK_WEBKIT=4.1
        # on such hosts (see issues #8 and #9).
        if ! pkg-config --exists webkit2gtk-4.0 && pkg-config --exists webkit2gtk-4.1; then
            WEBKIT_PC_DIR="$(pkg-config --variable=pcfiledir webkit2gtk-4.1)"
            WEBKIT_PC_ALIAS_DIR="$(mktemp -d)"
            ln -s "${WEBKIT_PC_DIR}/webkit2gtk-4.1.pc" "${WEBKIT_PC_ALIAS_DIR}/webkit2gtk-4.0.pc"
            export PKG_CONFIG_PATH="${WEBKIT_PC_ALIAS_DIR}${PKG_CONFIG_PATH:+:${PKG_CONFIG_PATH}}"
            trap 'rm -rf "${WEBKIT_PC_ALIAS_DIR}"' EXIT
            echo "Note: building default 4.0 path on a 4.1-only host via pc alias; for Ubuntu 24.04+/Mint 22.x use WA_DESK_WEBKIT=4.1 instead."
        fi
    fi
fi

if [ "${WA_DESK_USE_EXISTING_OUTPUT:-0}" = "1" ]; then
    # Packaging-only mode is useful when a CI runner receives a tested Linux
    # binary from a separate build job (for example, to create an RPM).
    test -x "${OUTPUT_DIR}/${APP_NAME}"
    test -f "${OUTPUT_DIR}/${APP_NAME}.desktop"
    test -f "${OUTPUT_DIR}/icon.png"
    echo "Reusing existing Linux build output for packaging."
else
    rm -rf "${OUTPUT_DIR}" "${APP_NAME}"
    mkdir -p "${OUTPUT_DIR}"

    echo "Compiling Linux binary..."
    go build -ldflags="-s -w -buildid= -X main.appVersion=${VERSION}" -trimpath -o "${OUTPUT_DIR}/${APP_NAME}" .

    # Copy assets
    cp icon.png "${OUTPUT_DIR}/"

    # Bundle optional shell installers with the portable archive. Package
    # managers (DEB/RPM) remain the preferred system-install path.
    install -m 0755 installer/linux/install.sh "${OUTPUT_DIR}/install.sh"
    install -m 0755 installer/linux/uninstall.sh "${OUTPUT_DIR}/uninstall.sh"

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
fi

# Create tar.gz release bundle
echo "Creating release tarball (WhatsApp-Desk-Linux-${BUNDLE_ARCH}${ARTIFACT_SUFFIX}.tar.gz)..."
TAR_BUNDLE="WhatsApp-Desk-Linux-${BUNDLE_ARCH}${ARTIFACT_SUFFIX}.tar.gz"
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
Depends: ${DEB_DEPENDS}
Section: net
Priority: optional
Description: Lightweight WhatsApp Desktop Client
 Independent, fast, and privacy-focused WhatsApp desktop application
 built with Go and native WebKit.
EOF

    dpkg-deb --build "${DEB_DIR}" "whatsapp-desk_${VERSION}_${DEB_ARCH}${ARTIFACT_SUFFIX}.deb"
    cp "whatsapp-desk_${VERSION}_${DEB_ARCH}${ARTIFACT_SUFFIX}.deb" "WhatsApp-Desk-Linux-${DEB_ARCH}${ARTIFACT_SUFFIX}.deb"
    rm -rf "${DEB_DIR}"
    echo "Created: whatsapp-desk_${VERSION}_${DEB_ARCH}${ARTIFACT_SUFFIX}.deb and WhatsApp-Desk-Linux-${DEB_ARCH}${ARTIFACT_SUFFIX}.deb"
fi

# Build a native Fedora/RHEL package when rpmbuild is available. The portable
# tarball remains the updater payload because it works across both Fedora and
# Debian-family releases; the RPM is for normal system installation.
if command -v rpmbuild >/dev/null 2>&1 && { [ "${WA_DESK_BUILD_RPM:-0}" = "1" ] || [ -f /etc/fedora-release ]; }; then
    echo "Building Fedora/RHEL package (.rpm)..."
    RPM_TOPDIR="${PWD}/rpmbuild"
	SOURCE_ROOT="${PWD}"
    rm -rf "${RPM_TOPDIR}"
    mkdir -p "${RPM_TOPDIR}/BUILD" "${RPM_TOPDIR}/BUILDROOT" "${RPM_TOPDIR}/RPMS" \
             "${RPM_TOPDIR}/SOURCES" "${RPM_TOPDIR}/SPECS" "${RPM_TOPDIR}/SRPMS"
    cat > "${RPM_TOPDIR}/SPECS/${APP_NAME}.spec" << EOF
Name:           ${APP_NAME}
Version:        ${VERSION}
Release:        1%{?dist}
Summary:        Lightweight WhatsApp desktop client
License:        MIT
BuildArch:      $( [ "${GOARCH_VALUE}" = "arm64" ] && echo aarch64 || echo x86_64 )
Requires:       gtk3
Requires:       webkit2gtk4.1

%description
Independent WhatsApp desktop client built with Go and the native WebKit engine.

%install
mkdir -p %{buildroot}/usr/bin %{buildroot}/usr/share/applications %{buildroot}/usr/share/icons/hicolor/512x512/apps
install -m 755 ${SOURCE_ROOT}/${OUTPUT_DIR}/${APP_NAME} %{buildroot}/usr/bin/${APP_NAME}
install -m 644 ${SOURCE_ROOT}/${OUTPUT_DIR}/${APP_NAME}.desktop %{buildroot}/usr/share/applications/${APP_NAME}.desktop
install -m 644 ${SOURCE_ROOT}/icon.png %{buildroot}/usr/share/icons/hicolor/512x512/apps/${APP_NAME}.png

%files
/usr/bin/${APP_NAME}
/usr/share/applications/${APP_NAME}.desktop
/usr/share/icons/hicolor/512x512/apps/${APP_NAME}.png
EOF
    rpmbuild -bb "${RPM_TOPDIR}/SPECS/${APP_NAME}.spec" --define "_topdir ${RPM_TOPDIR}"
    RPM_ARCH="$( [ "${GOARCH_VALUE}" = "arm64" ] && echo aarch64 || echo x86_64 )"
    cp "${RPM_TOPDIR}/RPMS/${RPM_ARCH}/${APP_NAME}-${VERSION}-1"*.rpm "WhatsApp-Desk-Fedora-${BUNDLE_ARCH}${ARTIFACT_SUFFIX}.rpm"
    rm -rf "${RPM_TOPDIR}"
    echo "Created: WhatsApp-Desk-Fedora-${BUNDLE_ARCH}${ARTIFACT_SUFFIX}.rpm"
fi

echo "Done! Linux artifacts ready in ${OUTPUT_DIR} and ${TAR_BUNDLE}."
