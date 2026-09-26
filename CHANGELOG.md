# Changelog

All notable changes to WhatsApp Desk are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

### Versioning scheme

WhatsApp Desk uses a **four-segment version** (`MAJOR.MINOR.PATCH.REVISION`, e.g. `1.5.9.7`).
This is a documented deviation from [Semantic Versioning](https://semver.org/spec/v2.0.0.html),
which permits only three segments. The first three segments keep their SemVer meaning; the
fourth is a **revision** counter used for consecutive releases that do not change the public
surface. Three-segment versions (for example `1.5.9`) are the earlier releases and remain valid.

Version numbers are declared in exactly one place — `appVersion` in `updater.go` — and
`release.sh` refuses to publish a release whose version does not match it.

---

## [Unreleased]

### Fixed

- Keep the Archived navigation row visible while blurring actual chat rows in Privacy Mode (#50).
- Linux: the self-updater prefers the `-webkit4.1` tarball on systems without the
  WebKitGTK 4.0 runtime (Ubuntu 24.04+, Mint 22.x, Fedora 39+), falling back to the
  historical artifact otherwise and never across architectures. Related to #8, #9.

## [1.5.9.8] - 2026-09-22

> Click Recovery, Multi-File Upload Hardening & WebKitGTK 4.1

Six pull requests contributed by [@MAliffadlan](https://github.com/MAliffadlan) land here
together, with the macOS and spreadsheet-preview follow-ups they needed to be mergeable.

### Security

- Self-update downloads are restricted to release artifacts of this repository served over
  HTTPS from `github.com`, so a page script can no longer point the updater at an arbitrary
  URL (#44).
- The download folder is validated against system and autostart locations, including symlink
  escapes and dangling symlinks, and the open-file bridge is jailed to the download folder and
  the internal preview directory (#44).
- Attacker-controlled strings in the in-app document preview are HTML-escaped (#47), and
  spreadsheet cell values are sanitized before they reach the preview table — SheetJS escapes
  cell text but writes the raw value into a `data-v` attribute, which a crafted cell could use
  to inject markup (#47 follow-up).
- Self-update archives no longer propagate elevated mode bits on extraction, and the DBus tray
  handler passes its error message as a format argument rather than as the format string
  (#45, #46).
- Self-update downloads are capped at 512 MB and saved attachments at 1 GB, checked before
  decoding so an oversized payload cannot exhaust memory (#49).

### Fixed

- Linux: opt-in WebKitGTK 4.1 build variant (`WA_DESK_WEBKIT=4.1`) for Ubuntu 24.04,
  Linux Mint 22.x and current Fedora, with `-webkit4.1` artifacts and honest DEB
  `Depends`. The default 4.0 build is unchanged. Related to #8, #9.
- Linux: the StatusNotifierItem tray properties no longer abort the process on launch on
  desktops with a tray watcher (#48).
- `wa_crash.log` rotates past 1 MB with a single backup, so a crash loop can no longer grow it
  without bound (#49).
- macOS: the download-folder blocklist and the preview-directory check resolved only one side
  of the comparison, so symlinked prefixes such as `/etc` and `/var` were never matched
  (follow-up to #44).
- A file drop that hit an excluded target (a dialog, the settings modal) or arrived with an
  empty file list — cloud placeholder files such as OneDrive's are the common case — left the
  drag-over highlight class stuck. That class disables pointer events across the whole app, so
  clicks stopped responding (including selecting a contact from the @mention popup) while
  typing and Enter kept working, until the app was restarted. The drag state is now reset on
  every drop and again on `dragend` and window blur.
- The drop fallback that waits for WhatsApp's native editor used a single 400 ms check; when
  the editor mounted slower than that — common on Windows — the app injected the dropped files
  a second time over the batch WhatsApp had already accepted, which could leave only one file
  in the editor. The fallback now probes for several rounds and only injects when no editor
  appeared during the whole window.

---

## [1.5.9.7] - 2026-09-19

> Notification Controls, Download Reliability & macOS Fixes

A consolidation release: 24 pull requests merged, 20 of them contributed by
[@agas007](https://github.com/agas007).

### Added

- Cross-platform notification controls in Settings, so native desktop notifications can be
  enabled or disabled consistently on macOS, Windows, and Linux (#31).
- Community documentation: [CONTRIBUTING.md](CONTRIBUTING.md) (#34),
  [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) — Contributor Covenant (#35),
  [SECURITY.md](SECURITY.md) with a vulnerability reporting policy (#36), and GitHub issue and
  pull request templates (#37).

### Changed

- macOS notifications now use the `UserNotifications` framework instead of the deprecated
  `NSUserNotification` API (#25).
- The duplicate-download toast reports "File already saved" instead of implying a fresh save (#40).
- Saved-state badges follow the filename element, so they stay aligned with their message
  bubble (#28).
- Settings shortcut button spacing corrected (#38).
- Releases are published from the local build pipeline rather than the tag-triggered workflow.

### Fixed

- Attachments keep their original filenames instead of falling back to a generic placeholder (#27).
- Downloading the same file twice no longer produces `name (1).ext` copies; files are matched by
  SHA-256 content hash and the existing path is reused (#29).
- Explicit context-menu downloads no longer open the in-app document viewer (#18).
- macOS: camera and microphone denial now surfaces a recovery card with a link to System Settings
  and a retry action (#30).
- macOS: media permissions are scoped to WhatsApp instead of being requested more broadly (#21).
- macOS: closing a chat with <kbd>Esc</kbd> no longer drops the window out of fullscreen (#24).
- macOS: closing a PDF preview no longer closes the underlying chat (#17).
- macOS: file picker uploads skip the native preview (#16).
- macOS: the "Launch at Startup" toggle reflects the real system state on every launch (#20).
- macOS: safer self-updater restart (#19).
- Privacy Mode: hovering a chat row reveals only that row, and sidebar timestamps blur
  consistently while staying readable (#22).
- Updater: failed updates report a specific reason instead of failing silently (#32).

### Removed

- Dead size-based download de-duplication bookkeeping left over from the previous
  implementation (#39).

---

## [1.5.9.6] - 2026-09-18

> Enhanced Privacy Mode & Avatar Blur

Resolves [Issue #15](https://github.com/vianziro/Whatsapp-Dekstop/issues/15) by expanding Privacy
Mode and avatar blur coverage across modern WhatsApp Web DOM structures.

### Fixed

- Privacy Mode blur and profile-photo blur now apply to the Archived Chats drawer and filtered
  lists, not only the main chat list container.
- Profile photo variants are all covered: contacts currently posting a Status (avatar wrapped in
  a status-ring SVG `<image>`), chats with disappearing messages, and pinned or previously pinned
  chats with badge overlays and Stylex container wrappers.
- Default SVG avatars for contacts without a custom profile picture are now blurred.
- The conversation header (`#main header`) and group message bubble avatars are uniformly covered.
- Hover reveal is now symmetrical and instant whether hovering the chat row or the profile photo
  directly.
- Sidebar and Archived Chats timestamps remain readable.

---

## [1.5.9.5] - 2026-09-18

> Window & Settings Fixes

Fixes the two most-reported Windows issues.

### Fixed

- Maximize / Restore Down returns to the previous custom size instead of staying full-screen, and
  the maximized state is remembered across restarts (#14).
- "Launch at Startup" reflects the actual system state on every launch (#13).
- The Settings icon no longer shifts position.
- Toast messages always appear above the Settings panel.
- Only one Settings control is ever shown at a time.
- Privacy Mode: gray redaction replaced with a visual blur, and chat list hover unblur corrected.
- Drag-and-drop for images restored by removing `stopImmediatePropagation`.

### Changed

- README and hero banner made version-agnostic so they no longer need editing every release.

---

## [1.5.9.4] - 2026-09-17

> Settings Panel Stability Fix

### Added

- Injected module isolation (`waRunModule`): every runtime feature (responsive CSS,
  appearance/themes, Privacy Mode, media observers, spellcheck, updater, settings) is isolated so
  an unexpected DOM error in one module can no longer abort the others or break the Settings modal
  and keyboard shortcuts.

### Fixed

- Settings panel crash on Windows WebView2 (#12): `init-body: Failed to execute 'observe' on
  'MutationObserver': parameter 1 is not of type 'Node'`, caused by pre-DOM script execution before
  `document.documentElement` exists.
- Drag-and-drop uploads stabilized on macOS and Windows: dropped documents (PDF, Office) are routed
  to the document input, media to the media input, and the sticker input is excluded.
- Windows: Settings gear visibility and keyboard shortcuts stabilized.
- Windows Setup wizard: branded NSIS installer compiled and onboarding polished.

---

## [1.5.9.3] - 2026-09-17

> Windows Installer & Single Settings Control

### Added

- Windows Setup wizard: branded graphics, Start Menu and desktop shortcuts, an Apps & features
  entry, and a clean uninstaller. Installs per-user, so no administrator prompt and no repeated
  elevation on updates. The portable EXE is still published.
- Linux portable installers, with Fedora packaged against WebKitGTK 4.1.
- English product landing page.
- Local crash log and optional debug logging via `WA_DESK_DEBUG=1`.
- 512 MB disk-cache budget.

### Changed

- Only a single Settings entry point is shown at a time; the header button, rail fallback, and
  last-resort launcher no longer appear together. A regression test runs the real injected script
  against a WhatsApp-shaped DOM and fails if two controls ever become visible at once.
- Window placement is remembered per monitor, and a saved frame from a disconnected monitor is
  never restored off-screen.
- Windows cross builds no longer require cgo.

### Fixed

- Windows: the page receives its own keyboard shortcuts.
- Windows: recovery when self-update replacement fails.
- Windows: the cache probe console window is hidden.
- Settings and appearance controls stabilized.

> **Note:** Linux packages are not part of this release.

---

## [1.5.9.2] - 2026-09-13

### Added

- Fedora package built against WebKitGTK 4.1.
- cgo-free cross builds for Windows.

### Changed

- Reduced background observer load.

---

## [1.5.9.1] - 2026-09-13

### Added

- Privacy polish: timestamp blur and header blackout.
- In-app issue reporter.
- `./build.sh check` — local three-OS build validation that does not depend on GitHub Actions.

---

## [1.5.9] - 2026-09-12

### Added

- Privacy Mode: text-shadow hiding with hover peek, plus an avatar blur setting.
- Linux: reliable always-on-top through GTK, and broader privacy selectors.

### Fixed

- Reverted the v1.5.8 CSP change that prevented the app from booting.
- Class-agnostic message bubble hide/restore via `msg-container`.
- Single-owner keyboard shortcuts and observer-war hardening.
- Windows and Linux: spellcheck persistence, tray specification, and monitor key.

---

## [1.5.8] - 2026-09-11

### Added

- Drag-and-drop uploads, spellcheck, and a native context menu.
- Idle privacy auto-lock.
- Monthly download folders.
- Actionable toasts.
- Per-monitor window memory.
- Crash log.
- Single version source.
- Saved badges on the media panel.
- Lazy-loaded SheetJS and CSP hardening.

### Changed

- Disk cache budget, observer-based injection, and optional debug logging.

### Fixed

- Privacy blur applied without transitions to avoid WebKitGTK resource exhaustion.
- Windows: cache purge deferred while the WebView2 engine is running.

---

## [1.5.7] - 2026-09-10

> Native File Picker on macOS

### Added

- Native file picker on macOS.

### Fixed

- Content-level download de-duplication.

---

## [1.5.6] - 2026-09-09

> Silent Background Update

### Changed

- Silent background update flow.

---

## [1.5.5] - 2026-09-09

> Theme and Performance Update

### Fixed

- Appearance/theme switching made reliable.
- Preview support for `ppt`, `xls`, and `doc`, and an infinite loading spinner that could hang.
- Completed downloads cached so previews reopen instantly.
- Duplicate attachment fetches prevented.
- Updater: update button guard and `onUpdateError` recovery in the UI.

### Changed

- WebView2 suspended when the window is minimized.
- Media scans limited to newly added DOM nodes.
- Real WebKit cache purge on suspend/resume, with throttled scroll observers.
- Reduced idle work and media buffering.

### Removed

- Resizable chat divider (reverted).

### Documentation

- Windows binary name standardized to `WhatsAppDesk.exe`.

---

## [1.5.4] - 2026-09-08

> PDF Viewer Lifecycle Fix

### Fixed

- Returning to the chat after closing a PDF.
- PDF downloads routed to the native viewer.
- Direct in-app PDF preview, with stuck background viewers dismissed.
- Status/story media loading and video autoplay.

### Changed

- All UI strings, menus, modals, and toasts localized to English, and documentation translated to
  full English.

---

## [1.5.3] - 2026-09-08

> Windows Black Screen Fix & macOS Menu Stability

### Added

- In-app preview for Excel (`.xlsx`, `.csv`) and Word (`.docx`, `.txt`).

### Fixed

- Windows 10/11 black screen on launch.
- macOS menu actions stabilized.
- Media viewer close button returns to the chat.

---

## [1.5.2] - 2026-09-08

> In-App PDF Preview Modal & Bug Fixes

### Added

- In-app PDF document modal, with a native system preview fallback.

### Fixed

- Stuck loading spinner auto-dismissed.

---

## [1.5.1] - 2026-09-08

> Bug Fixes & System Improvements

### Fixed

- Documents auto-open in the native preview app, with stuck viewers dismissed.
- Notification click focus, correct icons, and reliable theme switching across platforms.
- `blob:` `window.open` intercepted to launch the native viewer and in-app modal without breaking
  the main webview.
- In-chat PDF preview enabled via `pdfViewerEnabled`, and the click interceptor corrected.
- Smooth-scrolling GPU acceleration restored; the aggressive 60-second memory purge removed.
- macOS tray menu no longer triggers an assertion failure from duplicate status items.

### Changed

- Background update notifications enhanced.

### Performance

- Extreme RAM reductions on Windows and macOS.
- Linux WebKitGTK: memory, compositing, single-instance lock, and window state optimized.
- Windows: orphaned WebView2 processes prevented using Job Objects, with optimized memory limits.

---

## [1.5.0] - 2026-09-07

> Production Release & Memory Optimized

First production release under the WhatsApp Desk name.

### Added

- Persistent chat media download manager with a settings modal and native folder picker.
- In-app auto-updater with one-click update and background checks.
- Taskbar tray icon, toolbar button, and dark/light theme.
- Menu bar controls, floating button, and an interactive control center.
- Linux support.
- Comprehensive install and troubleshooting guides for macOS, Windows, and Linux.

### Changed

- Renamed to **WhatsApp Desk** and rebranded, with a redesigned production icon and updated icon
  badge.
- Memory and timer optimization.

### Performance

- WebKit caches optimized, canvas accelerated drawing disabled, and periodic purging added.
- Metal GPU async layer rendering enabled for smoother chat scrolling.
- Redundant disk I/O on Windows window state saving prevented.

---

## [1.4.0] - 2026-09-07

> Productivity Shortcuts & Native Enhancements

### Added

- Always-on-top, audio mute, auto-start, and reload shortcuts, each with HUD feedback.

### Changed

- Direct build dependencies locked and private build configuration protected.
- GitHub Actions workflows removed from the repository to prevent automated CI builds on forks.

---

## [1.3.1] - 2026-09-07

> Minimalist Onboarding & Visual Polish

### Changed

- Onboarding modal redesigned with a minimalist slow gradient and crisp SVG icons.

---

## [1.3.0] - 2026-09-07

> Onboarding Welcome Screen & Installer Visuals

### Added

- First-launch onboarding welcome modal with a visual illustration.
- DMG presentation background.

---

## [1.2.0] - 2026-09-07

> Dynamic Window Resizing & Responsive Layout

### Added

- Dynamic window resizing and a responsive layout on macOS and Windows.

---

## [1.1.1] - 2026-09-07

### Changed

- Direct download links added for the Windows `.exe` and macOS `.dmg`, and the release workflow
  updated.

---

## [1.1.0] - 2026-09-07

### Added

- macOS native WebKit support, performance optimizations, and cross-platform release CI.
- Windows external link binding.

### Changed

- README rewritten with balanced Windows and macOS coverage, advantages, benchmark comparison, and
  an MIT license.

---

## [1.0.1] - 2026-09-03

### Fixed

- WebView2 permission requests applied.

### Added

- Camera and microphone support for calls.

---

## [1.0.0] - 2026-09-03

### Added

- Initial release: a lightweight WhatsApp Web desktop application built on native OS web engines.

---

[Unreleased]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.5.9.8...HEAD
[1.5.9.8]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.5.9.7...v1.5.9.8
[1.5.9.7]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.5.9.6...v1.5.9.7
[1.5.9.6]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.5.9.5...v1.5.9.6
[1.5.9.5]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.5.9.4...v1.5.9.5
[1.5.9.4]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.5.9.3...v1.5.9.4
[1.5.9.3]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.5.9.2...v1.5.9.3
[1.5.9.2]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.5.9.1...v1.5.9.2
[1.5.9.1]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.5.9...v1.5.9.1
[1.5.9]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.5.8...v1.5.9
[1.5.8]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.5.7...v1.5.8
[1.5.7]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.5.6...v1.5.7
[1.5.6]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.5.5...v1.5.6
[1.5.5]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.5.4...v1.5.5
[1.5.4]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.5.3...v1.5.4
[1.5.3]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.5.2...v1.5.3
[1.5.2]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.5.1...v1.5.2
[1.5.1]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.5.0...v1.5.1
[1.5.0]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.4.0...v1.5.0
[1.4.0]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.3.1...v1.4.0
[1.3.1]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.3.0...v1.3.1
[1.3.0]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.2.0...v1.3.0
[1.2.0]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.1.1...v1.2.0
[1.1.1]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.1.0...v1.1.1
[1.1.0]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.0.1...v1.1.0
[1.0.1]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/vianziro/Whatsapp-Dekstop/releases/tag/v1.0.0
