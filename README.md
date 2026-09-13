# WhatsApp Desk

[![Release](https://img.shields.io/github/v/release/vianziro/Whatsapp-Dekstop?label=release)](https://github.com/vianziro/Whatsapp-Dekstop/releases/latest)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Windows%20%7C%20Linux-555)](https://github.com/vianziro/Whatsapp-Dekstop/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-555.svg)](LICENSE)

WhatsApp Desk is a small desktop wrapper for the official WhatsApp Web. It uses the web engine already provided by each operating system: WebKit on macOS, WebView2 on Windows, and WebKitGTK on Linux.

## Application preview

<p align="center">
  <img src="screenshots/app-dark.png" width="900" alt="WhatsApp Desk main chat window">
</p>

<p align="center">
  <img src="screenshots/macos-menu.png" width="650" alt="WhatsApp Desk settings and appearance controls">
</p>

The screenshots use blurred chat content to protect personal information.

## Download

Latest published release: **v1.5.9.1**

| Platform | Download |
| --- | --- |
| macOS, Apple Silicon and Intel | [DMG](https://github.com/vianziro/Whatsapp-Dekstop/releases/latest/download/WhatsApp-Desk-macOS-Universal.dmg) · [ZIP](https://github.com/vianziro/Whatsapp-Dekstop/releases/latest/download/WhatsApp-Desk-macOS-Universal.zip) |
| Windows 10/11 x64 | [EXE](https://github.com/vianziro/Whatsapp-Dekstop/releases/latest/download/WhatsAppDesk.exe) · [ZIP](https://github.com/vianziro/Whatsapp-Dekstop/releases/latest/download/WhatsApp-Desk-Windows-x64.zip) |
| Debian/Ubuntu x64 | [DEB](https://github.com/vianziro/Whatsapp-Dekstop/releases/latest/download/WhatsApp-Desk-Linux-amd64.deb) · [tar.gz](https://github.com/vianziro/Whatsapp-Dekstop/releases/latest/download/WhatsApp-Desk-Linux-x64.tar.gz) |
| Fedora/RHEL x64 | Use the [portable tar.gz](https://github.com/vianziro/Whatsapp-Dekstop/releases/latest/download/WhatsApp-Desk-Linux-x64.tar.gz) |

Linux arm64 packages will be published starting with v1.5.9.1. The updater only offers an architecture-compatible package; it never substitutes an x64 build on arm64.

## Main features

- Persistent WhatsApp Web session.
- Preview for PDF, Word, Excel, PowerPoint, CSV, and text files.
- Reopens files already downloaded without fetching them again.
- Configurable download folder.
- Privacy mode, always-on-top, audio mute, zoom, and theme controls.
- Native menus, notifications, and automatic update checks.
- Universal macOS build and portable Windows build.

RAM and CPU usage depend on the operating system, active chats, calls, media, and the web engine version.

## Installation

### macOS

1. Open the DMG.
2. Drag **WhatsApp Desk** into **Applications**.
3. If macOS blocks the first launch, right-click the app and choose **Open**.

The macOS build supports Apple Silicon and Intel. Camera and microphone permission will be requested only when needed.

### Windows

Download and run `WhatsAppDesk.exe`. Microsoft Edge WebView2 is required and is normally already installed on current Windows 10 and Windows 11 systems.

If SmartScreen appears, review the publisher warning, choose **More info**, and continue only if the file came from this repository's release page.

### Debian/Ubuntu

```bash
sudo dpkg -i WhatsApp-Desk-Linux-amd64.deb
sudo apt-get install -f
```

The portable archive requires GTK 3 and WebKitGTK 4.0 or 4.1.

## Shortcuts

| Action | macOS | Windows/Linux |
| --- | --- | --- |
| Settings | `Cmd + ,` | `Ctrl + ,` |
| Privacy mode | `Cmd + Shift + P` | `Ctrl + Shift + P` |
| Always on top | `Cmd + Shift + T` | `Ctrl + Shift + T` |
| Mute audio | `Cmd + Shift + M` | `Ctrl + Shift + M` |
| Open downloads | `Cmd + Shift + D` | `Ctrl + Shift + D` |
| Check updates | `Cmd + Shift + U` | `Ctrl + Shift + U` |
| Hard refresh | `Cmd + Shift + R` | `Ctrl + Shift + R` |

## Privacy

WhatsApp Desk loads `https://web.whatsapp.com` directly. Session and cache data remain inside the application's local profile. The application does not add an analytics service or a message relay server.

Errors and crash logs stay on your machine. Nothing is ever uploaded automatically: the Control Center's Report button (or the post-crash nudge) only opens a pre-filled GitHub issue in your browser, which you review before submitting.

Default profile locations:

- macOS: `~/Library/Application Support/WhatsAppDesk/UserData/`
- Windows: `%APPDATA%\WhatsAppDesk\UserData\`
- Linux: `~/.config/whatsapp-desk/`

## Version 1.5.9.1

- Linux update selection now distinguishes x64 and arm64, preventing an incompatible x64 download on arm64 devices.
- Release automation builds and publishes macOS, Windows x64, Linux x64, and Linux arm64 from the tagged source version.
- Removed the repository-tracked pseudo-secret build gate; it did not provide runtime security and could make a clean build fail unexpectedly.

## Version 1.5.9

- Critical fix: removed the v1.5.8 CSP policy that blocked WhatsApp boot bundles and left the app stuck on the splash screen.
- If you installed v1.5.8, update to v1.5.9 (in-app updater or fresh download).

## Version 1.5.8

- Native folder picker on Linux (GTK) and system tray with quick controls.
- Unified settings storage and extended WebView2 cache cleanup on Windows.
- Drag & drop files into chat, native spellcheck, and search/translate context menu.
- Taskbar progress badge on Windows and tray unread indicator on Linux.
- Hardened runtime on macOS, lazy spreadsheet engine, and CSP hardening.
- Unified `build.sh` and CI builds for macOS, Windows, and Linux.

## Version 1.5.7

- Attach menu works on macOS: Document and Photos & videos now open the native file picker.
- Files are no longer saved twice when a download is triggered from two paths.
- All v1.5.6 fixes included: silent background update on Windows (no console flashes), Fedora RPM packages, reliable document preview and appearance switching.

Older releases are retained for reference but are deprecated.

## License and disclaimer

Licensed under the [MIT License](LICENSE).

This is an independent project and is not affiliated with, authorized by, or endorsed by WhatsApp or Meta Platforms, Inc. WhatsApp is a trademark of its respective owner.
