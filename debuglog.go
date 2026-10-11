package main

// Optional internal diagnostics. Enabled with WA_DESK_DEBUG=1; everything is a
// no-op otherwise. Logs stay on this machine (profile dir / wa_debug.log) and
// are never shipped anywhere.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	debugLogMu     sync.Mutex
	debugLogFile   *os.File
	debugStartedAt = time.Now()
)

// debugLogMaxBytes caps the diagnostic log. Users are asked to send this file
// when they report a bug, so it grows exactly on the machines that most need it
// to stay readable — and it was opened for append with no cap at all. One
// previous generation is kept beside it.
const debugLogMaxBytes = 2 << 20 // 2 MiB

// debugMarkerPath is the opt-in file that enables diagnostics without an
// environment variable. A Finder/Dock launch cannot pass WA_DESK_DEBUG, and
// asking a user to run the binary from a terminal is a non-starter, so an empty
// file in the profile directory turns the same logging on; deleting it turns it
// off again. Nothing is uploaded either way.
func debugMarkerPath() string {
	return filepath.Join(filepath.Dir(debugLogPath()), "enable_diag")
}

func debugEnabled() bool {
	if os.Getenv("WA_DESK_DEBUG") == "1" {
		return true
	}
	_, err := os.Stat(debugMarkerPath())
	return err == nil
}

// diagLogFromPage records a page-side diagnostic (drag & drop, document
// preview, account switch) into the same local log. Gated by debugEnabled so
// the steady-state cost for normal users is a single stat call.
func diagLogFromPage(kind, detail string) bool {
	if !debugEnabled() {
		return false
	}
	kind = strings.TrimSpace(strings.ReplaceAll(kind, "\n", " "))
	detail = strings.TrimSpace(strings.ReplaceAll(detail, "\n", " "))
	if len(kind) > 40 {
		kind = kind[:40]
	}
	if len(detail) > 300 {
		detail = detail[:300]
	}
	cacheDebugLog("page %s: %s", kind, detail)
	return true
}

func debugLogPath() string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDirOrDot(), "Library", "Application Support", "WhatsAppDesk", "wa_debug.log")
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "WhatsAppDesk", "wa_debug.log")
	default:
		return filepath.Join(homeDirOrDot(), ".config", "whatsapp-desk", "wa_debug.log")
	}
}

func homeDirOrDot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

func cacheDebugLog(format string, args ...interface{}) {
	if !debugEnabled() {
		return
	}
	// A test run must never append to the real profile log. It resolves to the
	// same path, so `go test` used to interleave its own lines (e.g. an update
	// check with a fixture version) into the log a user is asked to read.
	if testing.Testing() {
		return
	}
	debugLogMu.Lock()
	defer debugLogMu.Unlock()
	if debugLogFile == nil {
		_ = os.MkdirAll(filepath.Dir(debugLogPath()), 0755)
		rotateDebugLog(debugLogPath())
		f, err := os.OpenFile(debugLogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		debugLogFile = f
	}
	fmt.Fprintf(debugLogFile, "[%s +%4.1fs] %s\n",
		time.Now().Format("15:04:05"), time.Since(debugStartedAt).Seconds(),
		fmt.Sprintf(format, args...))
}

// rotateDebugLog keeps the diagnostic log bounded: an oversized file is moved
// aside to "<name>.1", replacing any previous generation, so the next open
// starts a fresh one. Dropping the older half of a diagnostic session is the
// point — the alternative was a file that never stopped growing.
func rotateDebugLog(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < debugLogMaxBytes {
		return
	}
	_ = os.Remove(path + ".1")
	_ = os.Rename(path, path+".1")
}

// debugLogProcessStats writes the process's resident set size plus the goroutine
// count. The RSS is read from the operating system; see processRSSMB.
func debugLogProcessStats(phase string) {
	if !debugEnabled() {
		return
	}
	rss, source := processRSSMB()
	cacheDebugLog("stats phase=%s rss=%s source=%s goroutines=%d",
		phase, rss, source, runtime.NumGoroutine())
}

// processRSSMB reports this process's resident set size in megabytes as the
// operating system sees it, and names where the number came from. It does not
// use Go's runtime statistics: the heap the runtime has reserved is not the
// memory the OS has given the process, and reporting that made every rss= line
// in the log wrong by an order of magnitude — which is why the renderer's real
// growth had to be measured by hand instead of read from the log the app was
// already writing.
func processRSSMB() (string, string) {
	kb, source, ok := readProcessRSSKB()
	if !ok || kb <= 0 {
		return "n/a", "unavailable"
	}
	return fmt.Sprintf("%dMB", kb/1024), source
}
