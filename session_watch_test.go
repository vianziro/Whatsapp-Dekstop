package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withSessionLog points the session log at a temp file, resets the in-process
// state and returns the log path plus a clock the test can advance.
func withSessionLog(t *testing.T) (string, *time.Time) {
	t.Helper()
	withAccountRegistryFile(t)
	path := filepath.Join(t.TempDir(), "wa_session.log")
	originalPath, originalClock := sessionLogPath, sessionWatchClock
	now := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	sessionLogPath = func() string { return path }
	sessionWatchClock = func() time.Time { return now }
	forgetSessionStates()
	t.Cleanup(func() {
		sessionLogPath, sessionWatchClock = originalPath, originalClock
		forgetSessionStates()
	})
	return path, &now
}

// forgetSessionStates simulates a process restart.
func forgetSessionStates() {
	sessionWatchMu.Lock()
	defer sessionWatchMu.Unlock()
	sessionStates = map[string]sessionAccountState{}
	sessionPageLoads = 0
}

func readSessionLogLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

func TestSessionWatchLogsOnlyChangesAndFlagsALostSession(t *testing.T) {
	path, now := withSessionLog(t)

	if !recordSessionState(sessionStateSignedIn, "") {
		t.Fatal("the first signed-in report must be written")
	}
	*now = now.Add(time.Minute)
	if recordSessionState(sessionStateSignedIn, "") {
		t.Fatal("an unchanged state must not write a line")
	}
	*now = now.Add(2 * time.Minute)
	if !recordSessionState(sessionStateSignedOut, "A database error\n occurred.   Relink your device.") {
		t.Fatal("falling back to the QR page must be written")
	}

	lines := readSessionLogLines(t, path)
	if len(lines) != 2 {
		t.Fatalf("expected two lines (signed-in, signed-out), got %d: %q", len(lines), lines)
	}
	if !strings.Contains(lines[0], "state=signed-in") || strings.Contains(lines[0], "SESSION-LOST") {
		t.Errorf("unexpected first line: %q", lines[0])
	}
	for _, want := range []string{"state=signed-out", "was=signed-in", "for=3m0s", "SESSION-LOST", `detail="A database error occurred. Relink your device."`} {
		if !strings.Contains(lines[1], want) {
			t.Errorf("signed-out line is missing %q: %q", want, lines[1])
		}
	}
}

func TestSessionWatchSpotsASessionLostAcrossARestart(t *testing.T) {
	path, _ := withSessionLog(t)
	recordSessionState(sessionStateSignedIn, "")
	forgetSessionStates() // the app was quit and started again

	if recordSessionState(sessionStateSignedIn, "") {
		t.Fatal("still signed in after a restart is not a change")
	}
	forgetSessionStates()
	if !recordSessionState(sessionStateSignedOut, "") {
		t.Fatal("a session that is gone after a restart must be written")
	}
	lines := readSessionLogLines(t, path)
	last := lines[len(lines)-1]
	if !strings.Contains(last, "for=previous-run") || !strings.Contains(last, "SESSION-LOST") {
		t.Errorf("a loss across a restart must be flagged: %q", last)
	}
}

func TestSessionWatchCountsPageLoadsAndIgnoresUnknownStates(t *testing.T) {
	path, _ := withSessionLog(t)
	if recordSessionState("anything-else", "x") {
		t.Fatal("an unknown state must be ignored")
	}
	recordSessionState(sessionStatePageLoad, "")
	recordSessionState(sessionStatePageLoad, "")
	lines := readSessionLogLines(t, path)
	if len(lines) != 2 || !strings.Contains(lines[1], "state=page-load n=2") {
		t.Fatalf("page loads must be counted per run: %q", lines)
	}
	// A page load is not a session state: it must not hide a later loss.
	recordSessionState(sessionStateSignedIn, "")
	recordSessionState(sessionStatePageLoad, "")
	recordSessionState(sessionStateSignedOut, "")
	lines = readSessionLogLines(t, path)
	if !strings.Contains(lines[len(lines)-1], "SESSION-LOST") {
		t.Errorf("a loss after a reload must still be flagged: %q", lines[len(lines)-1])
	}
}

func TestSessionNoticesAreRecordedWithoutTheDebugLog(t *testing.T) {
	path, _ := withSessionLog(t)
	noteSessionEvent("the saved proxy could not be applied;\n WhatsApp is connecting %s", "directly")
	lines := readSessionLogLines(t, path)
	if len(lines) != 1 || !strings.HasSuffix(lines[0], "notice: the saved proxy could not be applied; WhatsApp is connecting directly") {
		t.Fatalf("a notice must be one clean line: %q", lines)
	}
	// The call sites that explain a lost session must use it, since
	// cacheDebugLog is a no-op unless diagnostics were switched on.
	darwin := readProjectFile(t, "app_darwin.go")
	for _, want := range []string{
		`noteSessionEvent("account registry unreadable (%v); started on the default profile", err)`,
		`noteSessionEvent("the saved proxy could not be applied; WhatsApp is connecting directly")`,
		`noteSessionEvent("WebKit data store for this account is missing`,
	} {
		if !strings.Contains(darwin, want) {
			t.Errorf("app_darwin.go no longer records %q in the always-on session log", want)
		}
	}
	if !strings.Contains(readProjectFile(t, "app_windows.go"), `noteSessionEvent("account registry unreadable (%v); started on the legacy user-data dir", dataDirErr)`) {
		t.Error("app_windows.go no longer records the legacy-profile fallback in the always-on session log")
	}
}

// The macOS idle reload must move the page to a fresh process. [wv reload]
// reloads inside the same WebContent process, which kept a long session at
// several GB even after repeated "memory" reloads.
func TestIdleReloadRecyclesThePageProcess(t *testing.T) {
	darwin := readProjectFile(t, "app_darwin.go")
	for _, want := range []string{
		"if (waDeskIdleRecycleBrowser()) {",
		"setIdleRecycle(func() bool {",
		"setIdleRecycle(nil)",
	} {
		if !strings.Contains(darwin, want) {
			t.Errorf("app_darwin.go is missing %q", want)
		}
	}
	// The recycle hook must be cleared before the WebView it captured is
	// destroyed, or the native timer could swap a dead view.
	clear := strings.Index(darwin, "setIdleRecycle(nil)")
	destroy := strings.Index(darwin[clear:], "w.Destroy()")
	if clear < 0 || destroy < 0 {
		t.Error("setIdleRecycle(nil) must come before w.Destroy() in the engine loop")
	}
	// It shares the account switch's lock and settle delay.
	hook := darwin[strings.Index(darwin, "setIdleRecycle(func() bool {"):]
	hook = hook[:strings.Index(hook, "\n\t\t})")]
	for _, want := range []string{"switchMu <- struct{}{}", "swapSettleDelay", "swapBrowserToActiveAccount(w)"} {
		if !strings.Contains(hook, want) {
			t.Errorf("the idle recycle hook must use %q", want)
		}
	}
}

func TestSessionWatchIsBoundOnEveryPlatformAndInThePage(t *testing.T) {
	for _, file := range []string{"app_darwin.go", "app_linux.go", "app_windows.go"} {
		if !strings.Contains(readProjectFile(t, file), `w.Bind("reportSessionStateNative", recordSessionState)`) {
			t.Errorf("%s does not bind reportSessionStateNative; a lost session would leave no trace there", file)
		}
	}
	script := getInitScript("UA")
	for _, want := range []string{"waRunModule('session-watch'", "report('page-load', '')", "window.top !== window"} {
		if !strings.Contains(script, want) {
			t.Errorf("the init script is missing %q", want)
		}
	}
}
