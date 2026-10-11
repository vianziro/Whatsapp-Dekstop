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

## [1.6.4] - 2026-10-11

> Session, Startup, Preview and Windows Tray Fixes

### Added

- Windows has a tray icon. macOS has had a status-bar item and Linux a StatusNotifierItem, so
  Windows was the one platform where closing the window ended the process and its notifications
  with it — the platform where a chat client is most expected to keep running. The close button now
  hides the window instead of quitting, a right-click on the tray offers Show, Hide and Quit, and a
  one-off notification says the app is still running so the window does not simply seem to have
  vanished. The tray is created on a message-only window of its own, so it does not depend on the
  browser window or on the vendored library's window procedure. It is deliberately fail-open: if
  the tray cannot be created the window is left completely alone and the close button quits exactly
  as before.
- Every push to `main` and every pull request is now built and tested by CI. It checks formatting,
  runs `go vet` on Linux — which also compiles `app_linux.go`, a file no local build can reach —
  cross-builds and vets the Windows target, and runs the test suite. The release workflow stays
  manual on purpose, for the reason documented in its own header.

### Changed

- The account rail no longer offers the add-account control once both accounts are present. With
  the cap at two, the control could never be used, and a permanently greyed-out button reads as a
  broken control rather than as a limit. It still appears while a slot is free, and returns as soon
  as one is released.

### Fixed

- The updater no longer offers an update whose download does not exist. The version check learns
  the latest version from the release page redirect, and the download URL was then built from a
  naming convention and never checked against the release itself — so on a platform with no
  published build the user was shown an update and sent to a link that answered HTTP 404 the
  moment they clicked it. The asset is now confirmed before it is advertised, and a release with no
  build for this platform says so instead of being presented as "up to date". Only a definite
  not-found counts: a timeout or a server error stays inconclusive, so a real update is never
  hidden.
- The diagnostic log is bounded. It was opened for append with no size cap, so it grew without
  limit on exactly the machines that were being asked to send it in a bug report. It now rotates
  past 2 MB, keeping one previous generation.
- The memory figure in the diagnostic log is real. It reported the heap the Go runtime had
  reserved rather than the memory the operating system had given the process, and it replaced the
  accurate reading on macOS and Windows, so every measurement in the log was wrong by an order of
  magnitude. It is now read from the operating system on each platform, and the log names where the
  number came from so a wrong figure is visible rather than plausible.
- The WhatsApp session can no longer be lost to the disk-cache trim on macOS. The 512 MB budget
  measured the whole `~/Library/WebKit/com.whatsapp.desk` tree, which is the session — cookies,
  localStorage and IndexedDB — so the cap tripped on ordinary session growth rather than on cache
  growth. The trim it then fired deleted `~/Library/Caches/com.whatsapp.desk` while the engine was
  still running, on every window minimise and every close, and that directory is not a pure cache:
  it holds WebKit's live service-worker registration database. The budget now measures and deletes
  HTTP caches only, from one shared list so the measured set and the deleted set cannot drift, and
  the delete happens once, before the engine starts. A target that never existed — the code looked
  for `NetworkCache` directly under the WebKit root — is fixed, so the trim also works now (#74).
- macOS handles SIGTERM and SIGINT. The app had no signal handler at all, so a logout, restart or
  shutdown killed it with writes still in flight and left nothing behind — which is why the
  symptom was indistinguishable from the app closing itself. Signals are now logged and the run
  loop ends through AppKit, the same path the Quit menu takes, so WebKit closes its website data
  store in order (#74).
- The macOS updater quits cleanly before its bundle is replaced. It exited abruptly, which skips
  WebKit's teardown, and the helper script that swaps the app waited a fixed one second — long
  enough to begin replacing the bundle while the session store was still open. The helper now
  waits for the process to actually disappear, bounded so a stuck process cannot block an update
  (#74).
- A missing WebView2 runtime now explains itself instead of crashing on Windows. The engine was
  created without checking for the runtime, and the vendored library leaves its internal interface
  pointer nil when the runtime is absent and then dereferences it — a panic that the startup
  recovery turned into a silent exit with no explanation. The runtime is checked first, and a
  missing one produces a dialog naming the download instead (#73).
- Switching accounts keeps the window where it was on Windows. The replacement window is created
  centred on the primary display, and the saved frame was looked up using *that* window's monitor,
  so a switch made from a secondary display moved the window to the primary one. The outgoing
  window's monitor is now carried across the switch. The switch also gets a visible transition
  again: the page fades in a veil before asking the native side to switch, and the teardown used to
  run in the same turn, so not one frame of it ever reached the screen (#72).
- Opening a PDF shows the in-app preview again. Clicking a document forwards the click to the
  document's own download control, which is how the file gets decrypted and fetched, and that
  forwarded click was being recorded as an explicit download. The explicit-download marker
  suppresses the preview by design, so every attempt to open a document fell through to WhatsApp's
  own viewer instead. A forwarded click no longer counts as a download, and a preview that is
  suppressed now records which guard suppressed it (#70).
- The Linux blank-window workaround is applied automatically. `WEBKIT_DISABLE_DMABUF_RENDERER=1`
  only took effect for launches from a shell, so it was lost the moment the app was started from a
  menu or a desktop entry. Setting it once now records it and every later launch keeps the
  fallback. It is also applied by itself when there is no DRM render node at all, which covers VMs,
  containers and remote desktops without a table of specific hardware. Machines that can composite
  in hardware are unaffected (#63).
- The microphone diagnostics actually record now. The capture probe read
  `navigator.mediaDevices` at document-creation time, when that API does not exist yet, and
  returned silently — so the `mic:` lines it was written to produce had never been written by any
  build, and every request for them came back empty. The probe retries until the API appears,
  within a bound, and records when it installs (#57).

---

## [1.6.3] - 2026-10-04

> Attachment Saving Fixed & Idle Memory Reload

### Added

- macOS: an optional **idle memory reload** frees the renderer's memory while the app is out of
  sight. WebKit's WebContent process keeps every media item the page has decoded — measured at
  ~2 GB on a long-lived session — and purging its memory cache reclaims only about 6% of that,
  because the rest is live page data WebKit is not allowed to drop. Once the window has been off
  screen for 15 minutes the page is reloaded, which releases the decoded images, the JavaScript
  heap and its typed arrays together. It only ever runs while the window is hidden, so an ordinary
  alt-tab cannot trigger it; it runs at most once every 30 minutes; and the page is asked first —
  `window.waIdleReloadAllowed()` refuses while offline, while an upload is in flight, while a
  document preview is open, or before the chat list has loaded. "Hidden" means out of sight —
  minimised, hidden, or fully covered by another window — not merely unfocused, so a window the
  user can still see is never reloaded underneath them. Switch it off under
  Settings → Maintenance.
- The idle-reload countdown can be shortened for a support session by writing the number of
  seconds into `idle_reload_seconds` in the app's profile folder
  (`~/Library/Application Support/WhatsAppDesk/`). The environment variable
  `WA_IDLE_RELOAD_SECONDS` does the same for a direct run, but macOS does not give a
  double-clicked bundle the shell's environment, so the file is the form a user can actually be
  asked to set. Either way the value must be a sane positive number of seconds, so it can never
  turn the feature into a reload loop. Each reload and each refusal is appended to
  `wa_idle_reload.log` in the same folder, and a refusal names the condition that blocked it
  ("the window is not hidden", "the chat list is not loaded", …) rather than leaving the reader to
  guess which of several situations applied.

### Fixed

- Saving an attachment no longer fails because of the name it was sent with. The filename comes
  straight from the chat and reached the filesystem with nothing removed but the path separators,
  so on Windows a document such as `Rapat 12:30.pdf` was rejected outright — `< > : " | ? *` are
  illegal there — and a name ending in a dot or space was silently renamed. Names are now cleaned
  of those characters, of control characters, of trailing dots and spaces, and of the reserved
  device names (`CON`, `NUL`, `COM1`…), consistently on every platform, so the same attachment is
  saved under the same name wherever the app runs (#68).
- A download folder that cannot be used no longer ends the save. If the configured folder was
  deleted, sits on a drive that is no longer mounted, or is refused by Windows' Controlled Folder
  Access, the attachment is written to the default folder instead of being lost, and the toast says
  where it actually went rather than reporting an ordinary success (#68).
- A failed save now explains itself. The native side knew why it refused — an unusable folder, a
  rejected name, a refused write — and threw the reason away, so the user saw only "Failed to save
  file." with nothing to act on and no way to report it. The reason is now shown in the toast and
  recorded in the page diagnostics (#68).

## [1.6.2] - 2026-09-28

> Windows Close & Account-Switch Fix

The 1.6.1 binaries were first published with a Windows bug that made the app impossible to close,
and the corrected build was re-published under the same 1.6.1 version number — which the updater
never offers to installs already on 1.6.1. 1.6.2 carries the same fixes under a new number so they
finally reach every 1.6.1 install.

### Fixed

- macOS: the app launches again on macOS 12 Monterey. The linker derived the bundle's minimum
  macOS from the build host's SDK (13.0), so dyld refused to load the binary on anything older and
  the documented "macOS 11.0+" requirement was silently false (#66, #67). The build now targets
  12.0 — the oldest release the Go 1.26 runtime supports — and Info.plist declares
  `LSMinimumSystemVersion`, so macOS warns before installing on an unsupported release instead of
  failing after it.
- Windows: the app closes normally again, and switching accounts no longer quits it. The session
  loop in `runApp` was `for !switchRequested` with the flag declared outside the loop, which put the
  two paths exactly the wrong way round: an ordinary close left the flag false, so the loop went
  round again and rebuilt the engine, reopening the window instead of exiting (#61, #64); a switch
  set the flag true, so the loop fell out and `runApp` returned, ending the process instead of
  rebuilding on the new account (#62). The flag is now declared per iteration and only an ordinary
  close leaves the loop, and the thread's message queue is drained after `Destroy()` so the
  `WM_QUIT` posted by the destroyed window's `WM_DESTROY` cannot reach the next session's `Run()`.
- Privacy Mode no longer makes scrolling heavy. The sidebar refresh asked every
  ancestor of the pointer target whether it was a sidebar control, and on a long
  chat list that read the whole list's text and re-queried its subtree on every
  pointer event; the refresh could also be dropped mid-flight, which risked leaving
  a chat row readable. The check is now bounded to the row it belongs to, and the
  refresh is deferred and rescheduled rather than dropped.
- Document previews scroll as smoothly as the built-in viewer. The preview overlay
  is a full-viewport fixed element, and blurring its backdrop forces the compositor
  to re-render the page behind the modal on every scrolled frame; the four scroll
  surfaces inside the card also repainted their whole overlay because nothing
  contained them. The overlay now uses a flat dim, every scroll surface is a
  containment boundary, and the card is layer-promoted.

## [1.6.1] - 2026-09-27

> Crash Fixes, Reliable Drag & Drop, and Document Preview Recovery

### Fixed

- Windows: the app no longer dies when the window is moved or activated while the
  webview is still being created. A title-bar click enters Windows' modal move loop,
  which re-entered the webview's message handler while its browser field was still
  empty, and the resulting fault killed the process (#58, #59, #60).
- macOS: the app no longer quits by itself when the account registry cannot be read
  at startup, or when the webview fails to build on the first attempt. Both paths
  ended the process; they now fall back to a single-account session and retry, and
  any remaining exit path leaves a crash report behind.
- Switching accounts is settled before another switch can start. A second switch
  requested while the previous swap was still painting could tear down the incoming
  view; switches now wait out the settle window.
- Document previews no longer reopen in a loop. Dismissing WhatsApp's own viewer
  makes it re-create the attachment, which re-entered the preview interceptor and
  reopened the window indefinitely; the same document is now auto-previewed once per
  window, and a fresh click is still honoured.
- Previewing a PDF no longer fights WhatsApp's viewer. The dismissal ran for 2.4
  seconds of close-button clicks and synthetic Escape keys; it is now bounded, and
  stops immediately when there is no viewer to dismiss.
- If the in-app PDF renderer cannot take a file, the preview card is shown instead
  of nothing, so there is always a way to open or save the document.
- Drag and drop works again for images and documents. The check that decided whether
  WhatsApp had accepted the drop treated any open dialog as success — and WhatsApp
  keeps dialog containers mounted permanently — so the fallback never ran and the
  drop silently did nothing.
- A second drop arriving while the first was still being staged no longer overwrites
  it.

### Added

- Capture diagnostics for calls (#57). The app now records the constraints the page
  requests and the settings the engine grants, so a "robotic microphone" report can
  be answered with data instead of a guess. The call path itself is untouched: the
  probe passes the original arguments and return value straight through.
- The in-app issue reporter now includes feature notes — decisions that changed what
  the user saw, such as a suppressed duplicate preview — next to page errors.

## [1.6.0] - 2026-09-27

> Multi-Account Profiles & a Quick-Tool Dock

### Added

- Account profiles: link a second WhatsApp account alongside the existing one.
  Each account keeps an isolated browser profile (WKWebsiteDataStore on macOS,
  a WebView2 user-data folder on Windows, a WebKitGTK website-data manager on
  Linux); the first account adopts the existing session by reference, so no
  re-pairing is needed. One engine is live at a time, and the account dock in
  the page rail switches, creates, and renames profiles, with
  `Ctrl/Cmd+Shift+1/2` as shortcuts. On macOS and Linux the switch swaps the
  browser view in place; on Windows it restarts the window.
- Unread hint per account chip: the dock shows the badge value an account had
  when it was last switched away from. Recorded once per switch, never polled.
- Quick-tool zone in the account dock: Privacy Mode, Always on Top, and Blur
  Profile Photos live as small icons below a divider under the account chips,
  with active-state coloring, tooltips, and the existing `Ctrl/Cmd+Shift+P/T`
  shortcuts still working. Purely event-driven — no polling.

### Fixed

- macOS 13 (and older): the second account now keeps its session across app
  restarts. The named per-account data store API is macOS 14+ only, so the
  fallback built an in-memory store before; it now builds the same persistent
  store through WebKit's `_WKWebsiteDataStoreConfiguration`. Account IDs from
  older registries (UUIDs without dashes) are canonicalized before parsing,
  which previously failed silently and downgraded every account to an
  in-memory store.
- Account switching no longer blanks the window. The replacement view is
  built before anything is touched on screen, the outgoing page stays visible
  (with a soft veil and clicks disabled) until the incoming account paints.

## [1.5.9.9] - 2026-09-26

> Privacy Reliability, Linux Self-Update & Update Banner Fixes

### Fixed

- Privacy Mode keeps the Archived navigation row and guidance visible while blurring actual
  archived chat rows, with localized Archived labels and bounded view-scoped scanning (#50, #52).
- Linux self-updates prefer the `-webkit4.1` tarball on systems without the WebKitGTK 4.0
  runtime, including GUI launchers whose PATH omits `ldconfig` locations, while preserving
  architecture-safe fallback behavior (#55).
- The update banner reserves layout space so it no longer covers the Archived header or back
  button, and disconnects its resize observer when dismissed (#56).

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

[Unreleased]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.5.9.9...HEAD
[1.5.9.9]: https://github.com/vianziro/Whatsapp-Dekstop/compare/v1.5.9.8...v1.5.9.9
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
