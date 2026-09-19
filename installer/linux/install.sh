#!/usr/bin/env bash
# WhatsApp Desk — Linux installer (run from inside the extracted release bundle).
# Usage:
#   tar -xzf WhatsApp-Desk-Linux-<arch>.tar.gz && cd dist_linux
#   sudo ./install.sh        # system-wide (/usr/local)
#   ./install.sh --user      # current user only (~/.local, no sudo)
set -e

APP_NAME="whatsapp-desk"
DISPLAY_NAME="WhatsApp Desk"
SRC_DIR="$(cd "$(dirname "$0")" && pwd)"

SYSTEM_PREFIX="/usr/local"
SYSTEM_BIN="${SYSTEM_PREFIX}/bin"
SYSTEM_APPS="${SYSTEM_PREFIX}/share/applications"
SYSTEM_ICONS="${SYSTEM_PREFIX}/share/icons/hicolor/512x512/apps"

USER_BIN="${HOME}/.local/bin"
USER_APPS="${HOME}/.local/share/applications"
USER_ICONS="${HOME}/.local/share/icons/hicolor/512x512/apps"

MODE="system"
for arg in "$@"; do
    case "${arg}" in
        --user) MODE="user" ;;
        --system) MODE="system" ;;
        -h|--help)
            echo "Usage: ./install.sh [--system] [--user]"
            echo "  --system  install to /usr/local (default, needs sudo)"
            echo "  --user    install to ~/.local (no sudo needed)"
            exit 0
            ;;
    esac
done

if [ ! -x "${SRC_DIR}/${APP_NAME}" ]; then
    echo "Error: ${APP_NAME} binary not found in ${SRC_DIR}." >&2
    echo "Run this script from inside the extracted release bundle." >&2
    exit 1
fi

if [ "${MODE}" = "user" ]; then
    BIN_DIR="${USER_BIN}"
    APPS_DIR="${USER_APPS}"
    ICONS_DIR="${USER_ICONS}"
    mkdir -p "${BIN_DIR}" "${APPS_DIR}" "${ICONS_DIR}"
    install -m 0755 "${SRC_DIR}/${APP_NAME}" "${BIN_DIR}/${APP_NAME}"
    DESKTOP_SRC="${SRC_DIR}/${APP_NAME}.desktop"
    if [ -f "${DESKTOP_SRC}" ]; then
        sed "s|^Exec=${APP_NAME}$|Exec=${BIN_DIR}/${APP_NAME}|" "${DESKTOP_SRC}" > "${APPS_DIR}/${APP_NAME}.desktop"
    else
        cat > "${APPS_DIR}/${APP_NAME}.desktop" <<EOF
[Desktop Entry]
Type=Application
Version=1.0
Name=${DISPLAY_NAME}
Comment=Lightweight WhatsApp Desktop Client
Exec=${BIN_DIR}/${APP_NAME}
Icon=${APP_NAME}
Terminal=false
Categories=Network;InstantMessaging;Chat;
StartupWMClass=${APP_NAME}
Keywords=WhatsApp;Chat;Messenger;
EOF
    fi
    if [ -f "${SRC_DIR}/icon.png" ]; then
        install -m 0644 "${SRC_DIR}/icon.png" "${ICONS_DIR}/${APP_NAME}.png"
    fi
    if command -v update-desktop-database >/dev/null 2>&1; then
        update-desktop-database "${APPS_DIR}" >/dev/null 2>&1 || true
    fi
    if command -v gtk-update-icon-cache >/dev/null 2>&1; then
        gtk-update-icon-cache -f -t "${HOME}/.local/share/icons/hicolor" >/dev/null 2>&1 || true
    fi
    echo "Installed ${DISPLAY_NAME} for the current user."
    echo "Binary: ${BIN_DIR}/${APP_NAME}"
    echo "Make sure ${BIN_DIR} is on your PATH."
else
    if [ "$(id -u)" -ne 0 ] && ! command -v sudo >/dev/null 2>&1; then
        echo "System install needs root. Re-run with sudo or use ./install.sh --user." >&2
        exit 1
    fi
    SUDO=""
    [ "$(id -u)" -ne 0 ] && SUDO="sudo"
    ${SUDO} install -d "${SYSTEM_BIN}" "${SYSTEM_APPS}" "${SYSTEM_ICONS}"
    ${SUDO} install -m 0755 "${SRC_DIR}/${APP_NAME}" "${SYSTEM_BIN}/${APP_NAME}"
    DESKTOP_SRC="${SRC_DIR}/${APP_NAME}.desktop"
    if [ -f "${DESKTOP_SRC}" ]; then
        ${SUDO} install -m 0644 "${DESKTOP_SRC}" "${SYSTEM_APPS}/${APP_NAME}.desktop"
    else
        TMP_DESKTOP="$(mktemp)"
        cat > "${TMP_DESKTOP}" <<EOF
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
        ${SUDO} install -m 0644 "${TMP_DESKTOP}" "${SYSTEM_APPS}/${APP_NAME}.desktop"
        rm -f "${TMP_DESKTOP}"
    fi
    if [ -f "${SRC_DIR}/icon.png" ]; then
        ${SUDO} install -m 0644 "${SRC_DIR}/icon.png" "${SYSTEM_ICONS}/${APP_NAME}.png"
    fi
    if command -v update-desktop-database >/dev/null 2>&1; then
        ${SUDO} update-desktop-database "${SYSTEM_APPS}" >/dev/null 2>&1 || true
    fi
    if command -v gtk-update-icon-cache >/dev/null 2>&1; then
        ${SUDO} gtk-update-icon-cache -f -t "${SYSTEM_PREFIX}/share/icons/hicolor" >/dev/null 2>&1 || true
    fi
    echo "Installed ${DISPLAY_NAME} system-wide."
    echo "Binary: ${SYSTEM_BIN}/${APP_NAME}"
fi
echo "Launch it from your app menu or by running: ${APP_NAME}"
