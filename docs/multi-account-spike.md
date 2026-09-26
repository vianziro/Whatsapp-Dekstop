# WhatsApp Desk — Multi-account Spike Memory

> Preserved from the spike worktree's `MEMORY.md`, which the repository
> gitignores. Committed here so the product decisions behind the
> multi-account branch stay alongside the code they describe.
>
> Status: **implemented on all three platforms (branch `feat/multi-account`).**
> macOS switches in place; Windows and Linux rebuild the engine on switch, with
> Linux also offering the in-place browser swap when the native extension is
> available. The five merge gates below are not yet met — manual validation on
> real accounts is still outstanding.

## Implementation status (2026-09-26)

- **macOS** — `WKWebsiteDataStore(forIdentifier:)` per UUID, selected before the
  `WKWebView` is created via the `WA_DESK_PROFILE_UUID` environment variable.
  Switching rebuilds only the browser view (`webview_recreate_browser_active()`)
  so the window, menu bar and tray survive; the teardown-rebuild loop is the
  fallback.
- **Windows** — one WebView2 user-data folder per account
  (`WA_DESK_PROFILE_DIR` is not used here; the Go side selects
  `activeAccountDataPath()` before the controller is created). The engine and
  its window are rebuilt per session (`runApp` loop); the research doc accepts
  this for the one-live-engine model because WebView2 named profiles would
  require forking the controller creation path.
- **Linux** — `WebKitWebsiteDataManager` + `WebKitWebContext` per profile
  directory, chosen from `WA_DESK_PROFILE_DIR` while the browser widget is
  created (`gtk_webkit_engine::set_up_web_view`). Switching prefers the same
  in-place widget swap as macOS (`recreate_browser_impl` for the GTK backend);
  the session rebuild loop is the fallback.
- **Shared** — `accounts.go` registry (max two accounts, opaque UUIDs, migration
  by reference for the first account), the account dock UI with
  `Ctrl/Cmd+Shift+1/2` shortcuts, and identical bridge semantics
  (`getAccountsNative`, `createAccountNative`, `renameAccountNative`,
  `requestAccountSwitchNative`) on every platform.

## Purpose

This worktree is an isolated technical/product spike for persistent dual-account
support. It must not destabilize the released single-account application in the
main worktree.

## Branch and baseline

- Branch: `codex/multi-account-spike`
- Baseline: `main` at `ad3d60f`
- Public release remains `v1.5.9.2`; do not create, replace, or upload release
  assets from this worktree unless explicitly requested.

## Product decision

- Support at most two saved WhatsApp accounts initially.
- Keep exactly one native WebView alive in normal operation.
- Switching accounts closes the current view, waits for teardown, then creates
  the selected account's isolated persistent profile.
- Do not ship a hidden second view, split chat canvas, unified inbox, or
  inactive-account notifications in the first version.

## Session isolation

- Never copy, parse, export, or merge cookies, tokens, IndexedDB, local
  storage, service workers, or WhatsApp message data between accounts.
- Account metadata must use opaque UUIDs and user-entered labels only.
- Web page JavaScript can request an account UUID but may never supply a path.
- Existing session data migrates in place as account `Default`; no forced QR
  re-pairing during migration.

## Platform direction

| Platform | Direction |
| --- | --- |
| Windows | Start with a custom WebView2 data root per selected account; consider named profiles under a shared UDF only after binding support and benchmarks. |
| macOS | Create persistent `WKWebsiteDataStore` per UUID before creating `WKWebView`. |
| Linux | Create `WebKitWebsiteDataManager` and `WebKitWebContext` per UUID before creating the view. |

## Performance gates

- One-account idle CPU/RAM must remain within 10% of the current stable baseline.
- No residual renderer/browser process after an account switch settles.
- Switching is serialized; a second request queues or is ignored while the
  current transition is running.
- Measure Windows x64, Intel and Apple Silicon macOS, Ubuntu/Debian, and Fedora
  before proposing a merge back to main.

## UX direction

- Use a compact account switcher in the application rail, not WhatsApp's
  volatile page header.
- Settings owns account labels, storage diagnostics, sign out, and removal.
- Account deletion first closes the profile and then moves owned storage to the
  OS Trash where supported.
- Preserve the project's restrained, native-looking interface; no dashboard,
  decorative cards, or extra long-lived browser views.

## Required gates before merge

1. Two-account pairing and restart preserve independent sessions.
2. Account A never displays Account B's chats, cache, or storage.
3. Ten repeated switches leave exactly one live engine.
4. PDF preview, downloads, updater, theme, privacy mode, and Settings work
   after switching on each supported platform.
5. Unit tests plus macOS, Windows, and Linux build checks pass.

## Reference

The detailed source-backed research is retained privately in the stable
worktree at:

`/Users/sepyankristanto/Documents/3.Data_Lainnya/whatsapp-web.view/docs/research/dual-account-product-architecture.md`

Keep the decision summary above current whenever this spike changes direction.
