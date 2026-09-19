<p align="center">
  <img src="screenshots/hero-banner.png" width="100%" alt="WhatsApp Desk - Native, Ultra-Light, Private Desktop Client">
</p>

<p align="center">
  <a href="https://github.com/vianziro/Whatsapp-Dekstop/releases/latest"><img src="https://img.shields.io/github/v/release/vianziro/Whatsapp-Dekstop?label=release&color=18c77b&style=flat-square" alt="Latest Release"></a>
  <a href="https://github.com/vianziro/Whatsapp-Dekstop/releases"><img src="https://img.shields.io/badge/platform-macOS%20%7C%20Windows%20%7C%20Linux-0b1713?style=flat-square" alt="Platforms"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-18c77b?style=flat-square" alt="License: MIT"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/go-1.26+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go Version"></a>
  <a href="https://github.com/vianziro/Whatsapp-Dekstop/releases/latest"><img src="https://img.shields.io/badge/architecture-Universal%20%7C%20x64-555?style=flat-square" alt="Architecture"></a>
</p>

<p align="center">
  <strong>WhatsApp Desk</strong> is a fast, ultra-lightweight, privacy-respecting desktop client for <a href="https://web.whatsapp.com">WhatsApp Web</a>.<br>
  Built with native operating system web engines — <strong>WebKit</strong> on macOS, <strong>WebView2</strong> on Windows, and <strong>WebKitGTK</strong> on Linux.<br>
  <em>Zero Electron bloat • Zero telemetry • Zero message relay servers • Complete local privacy.</em>
</p>

---

## ⚡ Downloads

Get the latest stable release for your operating system (updated automatically):

| Platform | Recommended Installer | Portable / Archive | Requirements |
| :--- | :--- | :--- | :--- |
| **macOS** | [**Universal DMG**](https://github.com/vianziro/Whatsapp-Dekstop/releases/latest/download/WhatsApp-Desk-macOS-Universal.dmg) | [Universal ZIP](https://github.com/vianziro/Whatsapp-Dekstop/releases/latest/download/WhatsApp-Desk-macOS-Universal.zip) | macOS 11.0+ (Apple Silicon & Intel) |
| **Windows 10 / 11** | [**Setup Wizard (.exe)**](https://github.com/vianziro/Whatsapp-Dekstop/releases/latest/download/WhatsApp-Desk-Windows-x64-Setup.exe) | [Portable EXE](https://github.com/vianziro/Whatsapp-Dekstop/releases/latest/download/WhatsAppDesk.exe) | Windows 10/11 x64 (WebView2 runtime) |
| **Linux** | [**Browse Releases (DEB / RPM)**](https://github.com/vianziro/Whatsapp-Dekstop/releases) | [tar.gz Archives](https://github.com/vianziro/Whatsapp-Dekstop/releases) | GTK 3 & WebKitGTK |

> [!TIP]
> Download links point automatically to the latest release assets. You can also view all past versions and architectures on the [Releases](https://github.com/vianziro/Whatsapp-Dekstop/releases) page.

---

## ✨ Key Features

* 🚀 **Ultra-Lightweight Engine:** Built directly on native OS webviews (WebKit on macOS, WebView2 on Windows, WebKitGTK on Linux). Minimal RAM and battery footprint compared to Chromium/Electron apps.
* 🛡️ **Zero Telemetry & Private by Design:** Communicates straight with `https://web.whatsapp.com`. No analytics tracking, no user profiling, and no proxy or relay servers.
* 👁️ **Instant Privacy Mode & Auto-Lock:** Quickly redact chat previews, sender names, and media thumbnails with a shortcut (`Ctrl+Shift+P` / `Cmd+Shift+P`) or automatic lock on idle.
* 📄 **Built-in Document & Office Preview:** Instant in-app previews for PDFs, Word docs, Excel spreadsheets, PowerPoint slides, and text attachments without cluttering your drive with duplicate files.
* 🛠️ **Windows Setup Wizard:** Per-user installer with branded artwork, Start Menu and desktop shortcuts, and clean uninstallation in Windows Apps & Features.
* ⚙️ **Unified Settings & Module Guard:** Single accessible settings control (`Ctrl+,` / `Cmd+,`) protected by runtime module isolation (`waRunModule`) against unexpected DOM changes.
* 🔄 **Built-in Self Updater:** Automatic update notifications with cryptographic `SHA256SUMS` validation before applying updates.
* 🖥️ **Per-Monitor Window Memory:** Automatically remembers window position and dimension across multi-monitor setups.

---

## 📸 Application Preview

<p align="center">
  <img src="screenshots/app-dark.png" width="900" alt="WhatsApp Desk Main Chat Window">
</p>

<p align="center">
  <img src="screenshots/macos-menu.png" width="620" alt="WhatsApp Desk Settings and Customization Panel">
</p>

*Note: Screenshots use blurred chat content to protect personal information.*

---

## ⌨️ Keyboard Shortcuts

| Feature | macOS | Windows & Linux |
| :--- | :--- | :--- |
| **Open Settings** | <kbd>Cmd</kbd> + <kbd>,</kbd> | <kbd>Ctrl</kbd> + <kbd>,</kbd> |
| **Toggle Privacy Mode** | <kbd>Cmd</kbd> + <kbd>Shift</kbd> + <kbd>P</kbd> | <kbd>Ctrl</kbd> + <kbd>Shift</kbd> + <kbd>P</kbd> |
| **Toggle Always on Top** | <kbd>Cmd</kbd> + <kbd>Shift</kbd> + <kbd>T</kbd> | <kbd>Ctrl</kbd> + <kbd>Shift</kbd> + <kbd>T</kbd> |
| **Mute / Unmute Audio** | <kbd>Cmd</kbd> + <kbd>Shift</kbd> + <kbd>M</kbd> | <kbd>Ctrl</kbd> + <kbd>Shift</kbd> + <kbd>M</kbd> |
| **Open Downloads Folder** | <kbd>Cmd</kbd> + <kbd>Shift</kbd> + <kbd>D</kbd> | <kbd>Ctrl</kbd> + <kbd>Shift</kbd> + <kbd>D</kbd> |
| **Check for Updates** | <kbd>Cmd</kbd> + <kbd>Shift</kbd> + <kbd>U</kbd> | <kbd>Ctrl</kbd> + <kbd>Shift</kbd> + <kbd>U</kbd> |
| **Hard Refresh Web View** | <kbd>Cmd</kbd> + <kbd>Shift</kbd> + <kbd>R</kbd> | <kbd>Ctrl</kbd> + <kbd>Shift</kbd> + <kbd>R</kbd> |
| **Zoom In / Out / Reset** | <kbd>Cmd</kbd> + <kbd>+</kbd> / <kbd>-</kbd> / <kbd>0</kbd> | <kbd>Ctrl</kbd> + <kbd>+</kbd> / <kbd>-</kbd> / <kbd>0</kbd> |

---

## 📦 Installation & Setup

### macOS (Universal)
1. Download [**WhatsApp-Desk-macOS-Universal.dmg**](https://github.com/vianziro/Whatsapp-Dekstop/releases/latest/download/WhatsApp-Desk-macOS-Universal.dmg).
2. Open the DMG and drag **WhatsApp Desk** into **Applications**.
3. *First launch:* If macOS Gatekeeper alerts you, right-click the application and select **Open**.

### Windows 10 / 11 (x64)
1. Download and run [**WhatsApp-Desk-Windows-x64-Setup.exe**](https://github.com/vianziro/Whatsapp-Dekstop/releases/latest/download/WhatsApp-Desk-Windows-x64-Setup.exe).
2. The setup wizard installs WhatsApp Desk per-user (no Administrator rights required) and adds Start Menu and desktop shortcuts.
3. *SmartScreen Note:* Since community binaries are unsigned (no EV certificate), choose **More info** → **Run anyway**.

### Linux (Debian / Ubuntu / Fedora)
```bash
# Debian / Ubuntu (x64)
sudo dpkg -i WhatsApp-Desk-Linux-amd64.deb
sudo apt-get install -f

# Fedora / RHEL (x64)
sudo dnf install ./WhatsApp-Desk-Fedora-x64.rpm
```

### Verifying Release Integrity

Every release ships with a signed [SHA256SUMS](https://github.com/vianziro/Whatsapp-Dekstop/releases/latest/download/SHA256SUMS) checksum list:

```bash
# macOS
shasum -a 256 -c SHA256SUMS

# Linux
sha256sum -c SHA256SUMS

# Windows PowerShell
(Get-FileHash .\WhatsApp-Desk-Windows-x64-Setup.exe -Algorithm SHA256).Hash.ToLower()
```

---

## Privacy

WhatsApp Desk loads `https://web.whatsapp.com` directly. Session and cache data remain inside the application's local profile. The application does not add an analytics service or a message relay server.

Errors and crash logs stay on your machine. Nothing is ever uploaded automatically: the Control Center's Report button (or the post-crash nudge) only opens a pre-filled GitHub issue in your browser, which you review before submitting.

Default profile locations:

- macOS: `~/Library/Application Support/WhatsAppDesk/UserData/`
- Windows: `%APPDATA%\WhatsAppDesk\UserData\`
- Linux: `~/.config/whatsapp-desk/`

---

## 📜 Release Notes & Changelog

Every version's changes are recorded in **[CHANGELOG.md](CHANGELOG.md)**, which follows the
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format. The published release notes are
generated from it, so the two can never disagree.

👉 **[Read the Changelog](CHANGELOG.md)** · **[View All Releases on GitHub](https://github.com/vianziro/Whatsapp-Dekstop/releases)**

---

## 🛠️ Building & Releasing

Releases are built and published from a local machine, not from hosted CI. This
keeps the release path independent of any runner account and makes every
published artifact reproducible on the maintainer's machine.

```bash
bash release.sh --check          # verify the toolchain
bash release.sh 1.5.9.7 --dry-run # build and checksum, publish nothing
bash release.sh 1.5.9.7          # build, publish, and verify the download
```

`release.sh` refuses to publish unless `updater.go` already declares the same
version, so a release can never advertise a version its binary does not report.
It builds the macOS universal DMG/ZIP and the Windows portable EXE, ZIP, and
Setup wizard, generates `SHA256SUMS` from the published asset list, and finally
re-downloads an artifact to verify its digest.

Individual targets are also available: `build_mac.sh`, `build_windows_installer.sh`,
and `build_linux.sh` (Linux packaging is currently maintained separately).

The `Release` workflow in `.github/workflows/build.yml` is manual-only for this
reason; its job definitions are kept for use on a host with working runners.

---

## License and disclaimer

Licensed under the [MIT License](LICENSE).

This is an independent project and is not affiliated with, authorized by, or endorsed by WhatsApp or Meta Platforms, Inc. WhatsApp is a trademark of its respective owner.
