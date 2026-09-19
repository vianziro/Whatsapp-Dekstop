# Dual-account feasibility: WhatsApp Desk

**Research date:** 2026-09-14. This is a design note only; it does not change account data, code, or releases.

## Decision

Two WhatsApp accounts are technically feasible as **two isolated persisted web sessions**, paired independently with WhatsApp. They are not feasible as WhatsApp Web's own account switcher: WhatsApp says account switching is currently for mobile companion devices and is not available on linked devices. The desktop app must own the account picker and isolate each browser profile. [WhatsApp Help Center](https://faq.whatsapp.com/492167569769444/?cms_platform=web)

Build **two saved profiles with one live WebView** first. Selecting an account destroys the current WebView and creates one against the selected profile. This retains login state and keeps normal idle memory close to today's single-account app. It does not promise notifications for an inactive account. Do not initially ship two concurrently live or hidden WebViews: they conflict with the project's CPU/RAM objective.

Each account needs its own WhatsApp pairing. The official phone-number linking flow applies to Web/Windows and the linked-device limit is currently four devices. [WhatsApp Help Center](https://faq.whatsapp.com/1324084875126592/?cms_platform=web&lang=en)

Never copy or inspect cookies, IndexedDB, local storage, service workers, or tokens across profile directories.

## Current checkout

| Platform | Current construction | Dual-profile implication |
| --- | --- | --- |
| Windows | `app_windows.go` supplies one `DataPath: userDataDir` to `go-webview2`. | One default session; `DataPath` can make separate roots but the pinned binding has no named controller-profile option. |
| macOS | `app_darwin.go` calls `webview.New(false)`. The vendored backend internally creates a default `WKWebViewConfiguration`. | The app currently uses WebKit's default persistent store. A per-profile store must be set before construction. |
| Linux | `app_linux.go` calls `webview.New(false)`; the vendored backend calls `webkit_web_view_new()`. | This uses the default WebKitGTK context/data manager rather than a profile-specific root. |

The common `webview_go` surface exposes `New`/`NewWindow`, not profile data-store options. A reliable cross-platform feature therefore needs a narrow native session factory per platform; it cannot be safely implemented only by injected JavaScript.

## Native engine capability

### Windows / WebView2

WebView2 supports both separate user-data folders (UDFs) and multiple named profiles under one UDF. Profiles isolate cookies, preferences, and website storage. Microsoft gives separate UDFs as its multi-account authentication example. [Authentication guidance](https://learn.microsoft.com/en-us/windows/apps/develop/ui/controls/webview2) · [CoreWebView2Profile](https://learn.microsoft.com/en-us/microsoft-edge/webview2/reference/winrt/microsoft_web_webview2_core/corewebview2profile?view=webview2-winrt-1.0.2592.51)

A compatibility path is one UDF per profile, e.g. `%APPDATA%\\WhatsAppDesk\\Profiles\\<id>\\WebView2`, which works with the current `DataPath` option. But Microsoft warns that each active UDF may add browser-process memory and disk usage; it recommends profiles under a shared UDF instead when multiple controls run concurrently. [Manage user data folders](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/user-data-folder)

For a later concurrent-account mode, extend the Windows binding to make controllers with deterministic `ProfileName` values under one shared UDF. Keep separate UDFs as fallback only if the installed runtime lacks named-profile support.

### macOS / WebKit

Apple documents that `WKWebsiteDataStore` owns cookies, cache, and other website data. `dataStoreForIdentifier:` creates a persistent store for profile browsing; it must be assigned to `WKWebViewConfiguration.websiteDataStore` before the WebView is created. `nonPersistentDataStore` is inappropriate for saved accounts because it does not survive restart. [WKWebsiteDataStore](https://developer.apple.com/documentation/webkit/wkwebsitedatastore?language=objc)

Expose a macOS factory that builds a configuration, assigns a stable per-account `dataStoreForIdentifier:`, then creates the `WKWebView`. The existing backend's internally-created configuration cannot be changed safely afterwards.

### Linux / WebKitGTK

WebKitGTK supports a `WebKitWebsiteDataManager` with construct-only `base-data-directory` and `base-cache-directory`, then a `WebKitWebContext` created with that manager. [WebsiteDataManager](https://webkitgtk.org/reference/webkit2gtk/stable/class.WebsiteDataManager.html) · [WebContext constructor](https://webkitgtk.org/reference/webkit2gtk/stable/ctor.WebContext.new_with_website_data_manager.html)

The Linux backend should create one manager/context per selected account, e.g. `~/.local/share/WhatsAppDesk/profiles/<id>/data` and `~/.cache/WhatsAppDesk/profiles/<id>/cache`, then create the WebKit view with that context. Do not use application-wide `XDG_*` environment changes as a profile switch mechanism.

## Resource trade-off

| Model | Isolation | Inactive notifications | Resource effect | Status |
| --- | --- | --- | --- | --- |
| One live WebView; switch profile | Strong | No, until switched back | Closest to current baseline | **Recommended first release** |
| Two live WebViews; one visible | Strong | Possible after per-profile native notification routing | Renderer/network/GPU work roughly doubles; extra UDFs are particularly costly on Windows | Later, opt-in only |
| Hidden second WebView | Strong | Possible | Highest constant CPU/RAM/network cost | Do not ship first |
| Two independent app processes | Strong | Possible | Duplicated lifecycle/update/download ownership | Do not use |

The current app has had RAM/CPU regressions. Measure idle and active memory, CPU, switching time, and disk use on Intel macOS, Windows x64, and Linux before accepting a concurrent mode. WebView2 explicitly documents the extra active-UDF resource cost. [Manage user data folders](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/user-data-folder)

## Staged architecture

### Phase 0 — safe foundation

- Add a versioned `accounts.json` in the existing settings root: opaque UUID, optional label, timestamps, selected ID. No WhatsApp credentials.
- Derive profile paths from a validated UUID only; never accept filesystem paths from web-page JavaScript.
- Preserve today's session as a `Default` profile; migration must not force QR re-pairing.
- Keep the existing single-instance lock. Multi-account is one app process, not several launchers racing on updates/downloads.

### Phase 1 — one active profile

- Place the account picker in native/app-owned UI, not WhatsApp's changing DOM.
- Add a `ProfileSessionFactory(profileID)` implementation for Windows, macOS, and Linux.
- Switching persists window state, destroys the active view, creates a view for the selected profile, re-injects existing enhancements, and navigates to WhatsApp Web.
- First use opens pairing; returning profiles must restore only their own session.
- `Sign out`/delete closes that view before clearing only that profile's owned storage. Deletion should be recoverable (move to Trash) and two-step.

**Acceptance tests:** pair two accounts; restart; switch repeatedly; prove Account A cannot show Account B's chats; sign out/delete B without affecting A; validate download/settings/update on all platforms; verify no two profile engines remain live in standard mode.

### Phase 2 — diagnostics and notifications

- Add profile label to native notifications only after a reliable per-profile bridge exists.
- Show active profile, storage size, and last switch without exposing messages, phone numbers, cookies, or tokens.
- Record platform performance baselines and regression budgets.

### Phase 3 — optional concurrent mode

- Windows uses named profiles under a shared UDF, not several simultaneous UDFs.
- macOS/Linux maintain explicit data-store/context per active WebView.
- Make it opt-in with a resource warning and a fallback to one live WebView under memory pressure.

## Implementation risks

1. The vendored generic backend needs a small maintained extension/fork on macOS and Linux for custom configuration/context creation; avoid duplicating the full library.
2. Switches must wait for native teardown before opening another engine to avoid leaked renderer processes.
3. WhatsApp pairing/session behavior can change independently; validate real devices on every major WhatsApp Web change.
4. Profile metadata, exports, and debug logs must never contain session secrets.

## Recommendation

Approve a private Phase-1 spike only: implement per-platform profile roots/factories and a hidden two-account selector, with exactly one WebView alive. Do not publish the user-facing feature until profile isolation, restart behavior, and cross-platform performance measurements all pass.
