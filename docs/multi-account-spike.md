# WhatsApp Desk — Multi-account Spike Memory

> Preserved from the spike worktree's `MEMORY.md`, which the repository
> gitignores. Committed here so the product decisions behind the
> multi-account branch stay alongside the code they describe.
>
> Status: **spike, macOS only.** The five merge gates below are not yet met.

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
