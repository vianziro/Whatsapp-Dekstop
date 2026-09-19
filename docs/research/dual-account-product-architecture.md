# Dual-account product architecture

## Decision

WhatsApp Desk should support **up to two saved accounts with one active web
session** in its first dual-account release. The account selector belongs to
the desktop shell, not the WhatsApp Web DOM. Selecting another account tears
down the current native web view, waits for its browser processes to close,
then creates one new view using only the selected account's persistent data
store.

This is a deliberate product constraint. It preserves the application's
single-account resource profile while providing genuine account isolation.
It also avoids advertising an unsupported WhatsApp Web account switcher:
WhatsApp account switching is not available on linked devices, so every saved
account must be paired separately. [1][2]

The first release must **not** keep a hidden second web view alive for instant
switching or background notification delivery. A WebView2 control can involve a
browser, renderer, GPU, audio, and utility processes; multiple active user-data
folders produce separate process collections. [3][4] On the current project,
where Windows and Intel macOS CPU/RAM regressions have already been a concern,
the feature is not acceptable if it trades a cleaner account switch for a
permanently doubled browser workload.

## Product scope

| In scope for v1 | Explicitly out of scope for v1 |
| --- | --- |
| Two persisted account slots | Two simultaneously live WhatsApp engines |
| Add, label, switch, sign out, remove account | Unified inbox or message search across accounts |
| Per-account WhatsApp session data | Copying cookies, IndexedDB, local storage, or tokens |
| Per-account notification identity while active | Background notifications from an inactive account |
| Per-account cache and storage diagnostics | Auto-reading a phone number or contact name from page storage |
| Account-specific privacy mode state | Parallel calls, media capture, or downloads |

The account label is user-entered, for example “Personal” and “Work”. The
application must not infer or save a telephone number merely to make the
picker look clever. This reduces exposure when a screenshot is shared and
avoids reliance on volatile WhatsApp page markup.

## UX model

### Primary layout: persistent account switcher in the rail

The selected account appears in the existing left rail, immediately above the
profile/avatar area and below the navigation controls. It is a compact native
button: a small initial or neutral avatar, the account label, and a chevron.
It has no decorative card, gradient, unread badge, or duplicated chat list.

Activating it opens a short menu:

| Row | Behaviour |
| --- | --- |
| `Personal` / `Work` | Switch to the chosen saved profile. The active profile is check-marked. |
| `Add account…` | Opens a native-owned pairing flow and creates an empty profile only after confirmation. |
| `Manage accounts…` | Opens the settings section for labels, storage, sign-out, and removal. |

The menu is keyboard accessible: focus starts on the selected account,
Up/Down moves among rows, Enter switches, Escape closes. The trigger has an
accessible label such as “Switch account, Personal”. On compact windows the
button remains icon-plus-tooltip rather than dropping below an unreliable DOM
injection point.

### Switching state

Switching is a state transition, not a progress spinner over the chat area.
Show a small native-owned interstitial with a single sentence:

```
Switching to Work
Your Personal session remains signed in on this device.
```

It has no cancel action after teardown begins. A cancellable transition is
misleading because the old WebView may already be closing. If the new engine
cannot start, the screen offers `Retry` and `Return to account list`; it never
silently opens the wrong profile or deletes either profile.

Expected states:

```text
Ready(A) -> Confirm switch -> Closing(A) -> Opening(B) -> Loading(B) -> Ready(B)
                                        \-> Failed(B) -> Account list
```

The settings dialog and updater are app-wide. Theme, startup behavior, window
geometry, generic download root, audio mute, and update settings remain
app-wide. The active-account label, profile cache size, last active timestamp,
and privacy-mode preference are account-scoped.

### Add-account flow

1. User chooses **Add account**.
2. A name field is optional. Default is `Account 2`; it can be renamed later.
3. The app creates a metadata record and an empty native profile directory.
4. The new WebView opens WhatsApp's standard linking screen. The user pairs
   the account on their phone.
5. Only after the page reaches a stable signed-in state does the app mark the
   account `ready`.

If pairing is closed or fails, the entry stays as `Needs pairing`; it is not
presented as a working account. The delete action requires a two-step
confirmation and first closes the profile's live view. Deletion moves the
profile directory to the operating system Trash when available, rather than
removing it recursively in place.

### Layout alternatives considered

| Layout | Benefit | Reason not selected for v1 |
| --- | --- | --- |
| Rail switcher | Familiar, low visual cost, works at narrow sizes, one chat canvas | **Selected** |
| Header account chips | Fast discoverability | Competes with WhatsApp's changing header and current Settings injection. |
| Two chat columns | Apparent simultaneous visibility | Requires two live engines; poor on narrow windows and doubles visual noise. |
| Account dashboard / unified inbox | Attractive product story | Requires indexing messages across accounts and introduces privacy, storage, and sync complexity. |
| Separate app windows | Strong separation | Duplicates updater, single-instance, notifications, and resource ownership. |

## Information architecture

Add an **Accounts** section to Settings, between Appearance and Downloads:

```text
Accounts
  Personal                         Active
  Last used today                  Switch

  Work                             Ready
  Last used yesterday              Switch

  + Add account

Storage and safety
  Account data is stored separately on this device.
  Manage storage…
```

`Manage storage` is intentionally a separate dialog. It shows only:

- account label;
- session status (`Ready`, `Needs pairing`, `Signed out`);
- profile data size and cache size;
- last active time;
- actions to clear cache, sign out, or remove.

It never displays database file names, cookies, tokens, messages, media
previews, or a raw filesystem path by default. An advanced diagnostic path can
reveal the profile directory only after a confirmation copy action.

## Session and storage architecture

### Metadata

Keep account metadata in an app-owned versioned JSON document adjacent to the
current settings file. It contains no web credentials:

```json
{
  "schema_version": 1,
  "active_account_id": "a6aa30e7-…",
  "accounts": [
    {
      "id": "a6aa30e7-…",
      "label": "Personal",
      "state": "ready",
      "created_at": "2026-09-14T12:00:00Z",
      "last_active_at": "2026-09-14T12:05:00Z",
      "privacy_mode": false
    }
  ]
}
```

IDs are generated UUIDs and validated before path construction. JavaScript may
ask to switch to an ID but can never supply a filesystem path. Reject labels
that are empty after trimming, overly long, or contain control characters; the
label is presentation data, not a directory name.

### Profile roots

| Platform | Recommended persistent root | Engine-specific isolation |
| --- | --- | --- |
| Windows | `%APPDATA%\\WhatsAppDesk\\Profiles\\<uuid>\\WebView2` | Start with one custom UDF per selected account; evolve to named profiles under one shared UDF only if the binding can expose controller profile options safely. |
| macOS | `~/Library/Application Support/WhatsAppDesk/Profiles/<uuid>` | Construct `WKWebViewConfiguration` with a persistent `WKWebsiteDataStore` identified by the UUID before creating the view. [5] |
| Linux | data: `~/.local/share/WhatsAppDesk/profiles/<uuid>`; cache: `~/.cache/WhatsAppDesk/profiles/<uuid>` | Construct a `WebKitWebsiteDataManager`, then its `WebKitWebContext`, before creating the view. [6][7] |

The present code has a single `getUserDataDir()` and uses the generic
`webview.New(false)` path on macOS and Linux. The generic binding does not
provide enough stable configuration surface for persistent per-profile stores.
The correct implementation is a small, tested `ProfileSessionFactory` behind a
platform interface, rather than JavaScript-based localStorage shuffling or a
global environment-variable switch.

```text
AccountService
  ├─ AccountStore        metadata, validation, migration
  ├─ ProfilePathPolicy   platform paths and cache budgets
  ├─ SessionFactory      new/close selected native WebView
  ├─ SessionCoordinator  serialized switch state machine
  └─ ProfileMaintenance  cache size, clear cache, safe removal
```

There is exactly one `SessionCoordinator` and one live `SessionFactory` result
in standard mode. A switch waits for destroy/close completion before creating
the next profile. On Windows that includes waiting for the previous WebView2
browser process that owns the UDF; Microsoft documents that user-data folders
must not be deleted while a session or its remaining browser processes are
still active. [4]

## Performance strategy

### Resource policy

1. **One active engine.** Do not prewarm the inactive profile by default.
2. **No profile cloning.** Existing default session becomes `Default` in
   place; copying live WebView folders risks corruption and duplicates cache.
3. **Independent cache budgets.** Apply the existing 512 MB disk-cache cap per
   active profile root, but track profile total size separately. Purging may
   remove only explicit cache directories, never cookies, IndexedDB, local
   storage, or service workers.
4. **Serialized lifecycle.** A second switch request queues behind the first;
   it does not create another window or WebView.
5. **No DOM polling for account state.** The shell records lifecycle state;
   page enhancements attach once after a stable navigation event.
6. **Visibility-aware work.** Existing scroll/visibility throttling continues
   to apply. Switching must cancel timers, mutation observers, and media
   preview work owned by the outgoing view.

On Windows, shared WebView2 environments and named profiles can reduce process
duplication only for a later concurrent mode. The current Go binding exposes a
`DataPath`, not the controller-profile API. Implementing named profiles must be
an intentional binding extension with runtime-version checks—not an assumed
optimization. Microsoft explicitly recommends avoiding redundant WebView2
instances and using a shared environment when multiple controls are genuinely
needed. [8]

### Performance budgets

Budgets are acceptance gates, not promises that all machines use identical
memory. Measure after a five-minute settled idle period and during a 30-minute
active-chat scenario with the same WhatsApp account/profile on each platform.

| Metric | v1 gate | Failure response |
| --- | --- | --- |
| Idle RSS / working set, one account | No more than 10% above current stable single-account baseline | Block release; capture engine process breakdown. |
| Idle CPU, one account | Median at or below current baseline; no sustained extra core use | Inspect timers, observers, and renderer/GPU processes. |
| Account-switch cold path | P95 below 8 seconds on supported baseline hardware | Show deterministic switch screen; optimize teardown/startup before prewarming. |
| Account-switch warm path | P95 below 5 seconds after both profiles have been paired | Same as above. |
| Residual outgoing renderer | Zero after switch settles | Block release; repair teardown. |
| Profile isolation | No session, cookie, local storage, IndexedDB, or cache crossover | Block release; do not ship. |
| Disk cache | Per-account cache remains within configured cap after restart | Fix path policy / maintenance. |

The benchmark sheet records machine model, OS/runtime version, WebView2 or
WebKit version, account chat count, conversation state, sample count, median,
P95, and the process tree. WebView2's documentation recommends profiling the
content/processes rather than attributing all CPU to the host by name. [9]

### Why not an instant switch?

An instant switch usually means retaining two active browser sessions. On
WebView2, each UDF is associated with a browser-process collection, and each
control may involve renderer and GPU work. [3][4] On WebKit, a second view also
retains page state, caches, networking, and GPU resources. A 2–8 second
deterministic switch is a better trade than a permanent cost on every user
machine. Revisit this only as a clearly labelled opt-in “keep both accounts
active” beta after resource measurements demonstrate a safe envelope.

## Notifications, downloads, and privacy

### Notifications

In v1, only the active account can produce notifications. Native notification
copy includes the user label when privacy mode is off, for example `Work — New
message`. With privacy mode on, it says only `Work — New activity` and never
includes contact or message text.

Do not attempt to scrape notification payloads from a hidden WebView. That
would violate the one-active-engine policy and create difficult cross-account
privacy behavior. An inactive account is marked `Inactive; notifications pause
until you switch back` in Manage Accounts.

### Downloads

The global download root remains a user setting. Save files under a stable,
user-visible account subdirectory only if the user enables **Separate downloads
by account**:

```text
WhatsApp Downloads/
  Personal/
  Work/
```

The default remains the current common download folder to avoid a surprising
path migration. Existing duplicate-download detection must include the target
account directory when separation is enabled; it must not treat a file in
Personal as proof that Work's unrelated attachment was downloaded.

### Privacy mode

Privacy mode is profile-scoped because it reflects the account's risk context.
The switch screen always redacts label-sensitive details while privacy mode is
enabled. Account labels remain visible to their owner so switching is possible,
but the user can select a neutral label such as `Account 1`.

## Failure handling and migration

| Condition | Required behaviour |
| --- | --- |
| Upgrade from current one-account version | Keep current data root unchanged and register it as `Default`; never force a QR scan. |
| New account fails pairing | Preserve an empty `Needs pairing` entry; allow retry or remove. |
| New profile starts blank after previously pairing | Offer diagnostic/retry; never copy data from another account. |
| Outgoing view refuses to close in time | Keep transition screen, log non-secret diagnostics, offer full app restart; do not create another engine. |
| Storage path unavailable | Do not start the view; give a path/permissions error and keep other accounts unchanged. |
| Account removal | Close it first, move owned storage to Trash, remove metadata only after success. |
| Upgrade rollback | Preserve `accounts.json`; unknown fields ignored and schema migrations are additive. |

## Delivery plan

### Phase A — foundation (private spike)

- Add `AccountStore`, versioned metadata, UUID validation, and migration of the
  existing default profile.
- Introduce a platform-neutral `SessionFactory` contract without changing the
  current one-account UI.
- Prototype one selected profile root per platform; validate pairing/restart
  manually on Windows, Intel macOS, Apple Silicon, Ubuntu/Debian, and Fedora.
- Establish the performance baseline before adding a second account.

### Phase B — two saved profiles, one active view

- Implement Account settings and rail switcher.
- Add serialized switch teardown/create logic and the transition screen.
- Add cache/storage diagnostics and account-safe remove/sign-out operations.
- Add regression tests for metadata, path traversal rejection, migration,
  teardown ordering, and per-platform session factory selection.

### Phase C — hardening

- Run 30-minute scenario tests on all platforms.
- Test QR pairing, restart, ten switches, downloads, PDF preview, themes,
  privacy mode, updater, and closing/reopening while switching.
- Instrument non-secret lifecycle durations and engine process counts only in
  diagnostics mode.

### Phase D — optional concurrent beta

This phase needs a separate design review. It may use named WebView2 profiles
under a shared UDF, explicit WebKit stores/contexts, opt-in notification
routing, memory-pressure fallback, and a hard maximum of two concurrent
accounts. It is not a follow-up checkbox for Phase B.

## Acceptance test matrix

| Test | Windows | macOS | Linux | Pass condition |
| --- | --- | --- | --- | --- |
| Pair Account A then B | Yes | Yes | Yes | Each opens its own WhatsApp session. |
| Restart on A, then switch B | Yes | Yes | Yes | No QR request for either paired profile. |
| Inspect storage | Yes | Yes | Yes | No shared cookies/IndexedDB/cache roots. |
| Ten successive switches | Yes | Yes | Yes | One live engine; no stuck loading screen. |
| PDF/download during and after switch | Yes | Yes | Yes | No duplicate download or cross-account file attribution. |
| Theme/privacy transition | Yes | Yes | Yes | Selected profile state applies without fuzzy blur or wrong theme. |
| Memory and CPU scenario | Yes | Intel + Apple Silicon | Ubuntu + Fedora | Meets performance budgets. |
| Sign out/remove B | Yes | Yes | Yes | A still works; B storage is only recoverably removed. |

## Recommendation

Approve Phase A as a private technical spike, then ship Phase B only when
profile isolation and baseline performance gates pass on every supported
platform. Keep the first public UX deliberately modest: rail switcher,
two saved labels, one active account. This provides a reliable practical
benefit without turning WhatsApp Desk into a permanently heavier browser.

## Sources

1. WhatsApp Help Center. [Switch accounts on WhatsApp](https://faq.whatsapp.com/492167569769444/?cms_platform=web). Accessed September 14, 2026.
2. WhatsApp Help Center. [Link a device with phone number](https://faq.whatsapp.com/1324084875126592/?cms_platform=web&lang=en). Accessed September 14, 2026.
3. Microsoft. [Process model for WebView2 apps](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/process-model). Accessed September 14, 2026.
4. Microsoft. [Manage user data folders](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/user-data-folder). Accessed September 14, 2026.
5. Apple. [WKWebsiteDataStore](https://developer.apple.com/documentation/webkit/wkwebsitedatastore?changes=_1_3&language=objc). Accessed September 14, 2026.
6. WebKitGTK. [WebKitWebsiteDataManager](https://webkitgtk.org/reference/webkit2gtk/stable/class.WebsiteDataManager.html). Accessed September 14, 2026.
7. WebKitGTK. [WebKitWebContext with WebsiteDataManager](https://webkitgtk.org/reference/webkit2gtk/stable/ctor.WebContext.new_with_website_data_manager.html). Accessed September 14, 2026.
8. Microsoft. [Performance best practices for WebView2 apps](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/performance). Accessed September 14, 2026.
9. Microsoft. [WebView2 end-user FAQ](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/end-user-faq). Accessed September 14, 2026.
