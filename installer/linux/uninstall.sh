#!/usr/bin/env bash
# WhatsApp Desk — Linux uninstaller.
# Removes files installed by install.sh (system or user mode).
# Usage:
#   sudo ./uninstall.sh        # remove system-wide install
#   ./uninstall.sh --user      # remove current-user install
set -e

APP_NAME="whatsapp-desk"

SYSTEM_BIN="/usr/local/bin/${APP_NAME}"
SYSTEM_DESKTOP="/usr/local/share/applications/${APP_NAME}.desktop"
SYSTEM_ICON="/usr/local/share/icons/hicolor/512x512/apps/${APP_NAME}.png"

USER_BIN="${HOME}/.local/bin/${APP_NAME}"
USER_DESKTOP="${HOME}/.local/share/applications/${APP_NAME}.desktop"
USER_ICON="${HOME}/.local/share/icons/hicolor/512x512/apps/${APP_NAME}.png"

MODE="system"
for arg in "$@"; do
    case "${arg}" in
        --user) MODE="user" ;;
        --system) MODE="system" ;;
        -h|--help)
            echo "Usage: ./uninstall.sh [--system] [--user]"
            exit 0
            ;;
    esac
done

if [ "${MODE}" = "user" ]; then
    rm -f "${USER_BIN}" "${USER_DESKTOP}" "${USER_ICON}"
    if command -v update-desktop-database >/dev/null 2>&1; then
        update-desktop-database "${HOME}/.local/share/applications" >/dev/null 2>&1 || true
    fi
    echo "Removed per-user ${APP_NAME} install (if present)."
else
    if [ "$(id -u)" -ne 0 ] && ! command -v sudo >/dev/null 2>&1; then
        echo "System uninstall needs root. Re-run with sudo or use ./uninstall.sh --user." >&2
        exit 1
    fi
    SUDO=""
    [ "$(id -u)" -ne 0 ] && SUDO="sudo"
    ${SUDO} rm -f "${SYSTEM_BIN}" "${SYSTEM_DESKTOP}" "${SYSTEM_ICON}"
    if command -v update-desktop-database >/dev/null 2>&1; then
        ${SUDO} update-desktop-database "/usr/local/share/applications" >/dev/null 2>&1 || true
    fi
    if command -v gtk-update-icon-cache >/dev/null 2>&1; then
        ${SUDO} gtk-update-icon-cache -f -t "/usr/local/share/icons/hicolor" >/dev/null 2>&1 || true
    fi
    echo "Removed system-wide ${APP_NAME} install (if present)."
fi
echo "Catatan: data profil pengguna (~/.config/whatsapp-desk) tidak dihapus."
