//go:build darwin

package main

// Idle memory reload with a fresh page process.
//
// The idle reload used to call [WKWebView reload], which loads WhatsApp again
// inside the SAME WebContent process. WebKit's allocator keeps the pages the
// old page dirtied, so a long session sat at 3+ GB (most of it pushed to swap)
// even after several "memory" reloads: the reload reset the page, not the
// process. Swapping the browser view - the in-place rebuild used for account
// switches - starts a new process and tears the old one down with its memory.
//
// This lives in its own file because cgo forbids //export in a file whose
// preamble defines C functions, which app_darwin.go does.

import "C"

import "sync"

var (
	idleRecycleMu sync.Mutex
	// idleRecycle swaps the live browser view for a fresh one and reports
	// whether it did. runApp sets it for the WebView of the current loop
	// iteration and clears it before that WebView is destroyed, so the native
	// timer can never reach a dead view.
	idleRecycle func() bool
)

func setIdleRecycle(fn func() bool) {
	idleRecycleMu.Lock()
	idleRecycle = fn
	idleRecycleMu.Unlock()
}

// waDeskIdleRecycleBrowser is called by the native idle-reload timer on the
// main thread. Non-zero means the page now runs in a new process; zero asks
// the caller to fall back to an in-process reload.
//
//export waDeskIdleRecycleBrowser
func waDeskIdleRecycleBrowser() C.int {
	idleRecycleMu.Lock()
	fn := idleRecycle
	idleRecycleMu.Unlock()
	if fn != nil && fn() {
		return 1
	}
	return 0
}
