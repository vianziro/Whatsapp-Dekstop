package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests pin the fixes made for the open bug reports. Each one names the
// issue it guards and fails with a message that says what breaks if the
// invariant is lost, because a guard that only says "want X" gets deleted
// instead of trusted.

// readProjectFile reads a source file that must exist. Test helper.
func readProjectFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("cannot read %s: %v", name, err)
	}
	return string(data)
}

// stripCommentLines removes whole-line comments so a guard can never be tripped
// by prose. A comment that merely *mentions* a call site (for example "the code
// after w.Run() is never reached") must not count as that call site existing, or
// the assertion fires on a correct build. Trailing comments on code lines are
// left alone on purpose: cutting from the first "//" would also truncate any
// line containing a URL.
//
// It lives here rather than in a platform-specific test file so every platform's
// build can reach it; it was darwin-only at first, which broke `GOOS=windows go
// vet`.
func stripCommentLines(src string) string {
	lines := strings.Split(src, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			kept = append(kept, "")
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// --- #63: WebKitGTK DMABUF fallback ---------------------------------------

func TestDMABUFDecision(t *testing.T) {
	renderNode := []string{"/dev/dri/renderD128"}

	cases := []struct {
		name         string
		envValue     string
		markerExists bool
		renderNodes  []string
		wantDisable  bool
		wantPersist  bool
		wantReason   string
	}{
		{
			// The user set the variable themselves. We must not set it again,
			// but we must record it, or their fix evaporates the next time the
			// app is started from a desktop entry instead of their shell.
			name: "explicit env var is remembered", envValue: "1",
			renderNodes: renderNode,
			wantDisable: false, wantPersist: true, wantReason: dmabufReasonEnv,
		},
		{
			// Already remembered: no need to write the marker a second time.
			name: "explicit env var with marker already present", envValue: "1",
			markerExists: true, renderNodes: renderNode,
			wantDisable: false, wantPersist: false, wantReason: dmabufReasonEnv,
		},
		{
			name: "marker from an earlier run", markerExists: true,
			renderNodes: renderNode,
			wantDisable: true, wantPersist: false, wantReason: dmabufReasonMarker,
		},
		{
			// No DRM render node means the DMABUF renderer has nothing to hand
			// buffers to. Applies to VMs, containers and remote desktops without
			// naming any specific hardware.
			name: "no render node", renderNodes: nil,
			wantDisable: true, wantPersist: false, wantReason: dmabufReasonNoRender,
		},
		{
			// Working hardware keeps hardware compositing.
			name: "hardware that can composite is left alone", renderNodes: renderNode,
			wantDisable: false, wantPersist: false, wantReason: dmabufReasonDefault,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			disable, persist, reason := decideDMABUFDisable(tc.envValue, tc.markerExists, tc.renderNodes)
			if disable != tc.wantDisable {
				t.Errorf("disable = %v, want %v: the wrong launch path is chosen for %s", disable, tc.wantDisable, tc.name)
			}
			if persist != tc.wantPersist {
				t.Errorf("persistMarker = %v, want %v", persist, tc.wantPersist)
			}
			if reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", reason, tc.wantReason)
			}
		})
	}
}

func TestLinuxStartupAppliesRenderingFallbackBeforeEngineStarts(t *testing.T) {
	src := readProjectFile(t, "app_linux.go")

	call := strings.Index(src, "linuxRenderingFallbacks(userDataDir)")
	if call < 0 {
		t.Fatal("runApp never applies the rendering fallback, so the blank-window workaround is never automatic")
	}
	// The variable has to be in the environment before the engine is created,
	// so the call must sit above the first webview construction.
	engine := strings.Index(src, "webview.New(")
	if engine < 0 {
		t.Fatal("cannot find where the Linux engine is created")
	}
	if call > engine {
		t.Error("the rendering fallback is applied after the engine is created, so it has no effect on this launch")
	}
}

// --- #57: capture diagnostics never installed ------------------------------

func TestCaptureProbeRetriesUntilMediaDevicesExists(t *testing.T) {
	script := getInitScript("test-agent")

	start := strings.Index(script, "waRunModule('capture-diagnostics'")
	if start < 0 {
		t.Fatal("the capture-diagnostics module is missing from the init script")
	}
	end := strings.Index(script[start:], "waRunModule('launch-on-boot'")
	if end < 0 {
		t.Fatal("cannot find the end of the capture-diagnostics module")
	}
	module := script[start : start+end]

	// The old version read navigator.mediaDevices once and gave up when it was
	// absent, which is why the mic: diagnostics issue #57 asks for were never
	// produced by any build. The probe must be installed from a retrying
	// installer instead.
	if !strings.Contains(module, "function installCaptureProbe()") {
		t.Error("the capture probe is not wrapped in a retrying installer, so it is skipped whenever mediaDevices is not ready at injection time")
	}
	if !strings.Contains(module, "setInterval(") {
		t.Error("the capture probe never retries: mediaDevices is frequently absent when the init script is injected, so the probe would never install")
	}
	if !strings.Contains(module, "captureProbeAttempts > 40") {
		t.Error("the capture probe retry is not bounded, so it could become a permanent timer")
	}
	// A silent early return is the defect itself.
	if strings.Contains(module, "|| md.__waCaptureProbe) return;") {
		t.Error("the probe still bails out silently instead of retrying")
	}
}

// --- #70: in-app document preview suppressed -------------------------------

func TestForwardedDocumentClickIsNotAnExplicitDownload(t *testing.T) {
	script := getInitScript("test-agent")

	// The explicit-download marker suppresses the in-app preview. Setting it in
	// more than one place is how a click on a document came to be recorded as a
	// download: opening a PDF then showed the built-in viewer instead of ours.
	const setter = "lastExplicitDownloadAt = Date.now()"
	if got := strings.Count(script, setter); got != 1 {
		t.Errorf("the explicit-download marker is set in %d places, want exactly 1 (inside markExplicitDownload): every other setter bypasses the forwarded-click check", got)
	}

	helper := strings.Index(script, "function markExplicitDownload()")
	if helper < 0 {
		t.Fatal("markExplicitDownload is missing, so the forwarded-click check has nowhere to live")
	}
	body := script[helper:]
	if end := strings.Index(body, "\n\t\t\t}"); end > 0 {
		body = body[:end]
	}
	if !strings.Contains(body, "forwardingDocumentDownload") {
		t.Error("markExplicitDownload does not skip our own forwarded document click, so opening a document is still recorded as a download and the preview stays suppressed")
	}
	// Every call site must go through the helper.
	if got := strings.Count(script, "markExplicitDownload()"); got < 3 {
		t.Errorf("only %d call sites use markExplicitDownload; a raw setter would skip the forwarded-click check", got)
	}
}

func TestSuppressedPreviewRecordsWhy(t *testing.T) {
	script := getInitScript("test-agent")
	// The two suppression reasons are indistinguishable to the user, so a
	// report of "the preview did not open" must carry the reason itself.
	if !strings.Contains(script, "preview skipped for") {
		t.Error("a suppressed document preview is not recorded, so a report of it cannot be answered without another round trip")
	}
}

// --- #74: session lost to an unclean exit ---------------------------------

func TestMacOSCachePurgeNeverRunsMidSession(t *testing.T) {
	src := readProjectFile(t, "app_darwin.go")

	start := strings.Index(src, "static void enforceDiskCacheCap(void) {")
	if start < 0 {
		t.Fatal("enforceDiskCacheCap is missing")
	}
	end := strings.Index(src[start:], "static void whatsappDeskRequestTerminate(void) {")
	if end < 0 {
		t.Fatal("cannot find the end of enforceDiskCacheCap")
	}
	midSession := src[start : start+end]

	// This used to delete the cache tree on every minimise and every window
	// close, while the engine still owned it -- and that tree carries WebKit's
	// live service-worker registration database.
	if strings.Contains(midSession, "removeItemAtPath") {
		t.Error("the mid-session cache check still deletes: it runs while the engine owns those files, which can corrupt the live store")
	}
	if !strings.Contains(src, "whatsappDeskPurgeDiskCacheBeforeLaunch") {
		t.Error("the pre-launch purge is missing, so the cache cap is never actually enforced")
	}
}

func TestMacOSCacheBudgetDoesNotCountSessionStore(t *testing.T) {
	src := readProjectFile(t, "app_darwin.go")

	start := strings.Index(src, "static NSArray* whatsappDeskDiskCacheDirs(void) {")
	if start < 0 {
		t.Fatal("whatsappDeskDiskCacheDirs is missing: the measured set and the deleted set must come from one list or they drift")
	}
	end := strings.Index(src[start:], "static long long whatsappDeskDiskCacheBytes(void) {")
	if end < 0 {
		t.Fatal("cannot find the end of whatsappDeskDiskCacheDirs")
	}
	dirs := src[start : start+end]

	// Counting the whole WebKit root towards a 512 MB "cache" budget made the
	// cap trip on ordinary session growth, and the purge it fired then deleted
	// files out from under a running engine.
	if strings.Contains(dirs, `stringByAppendingPathComponent:@"Library/WebKit/com.whatsapp.desk"]`) {
		t.Error("the cache budget still counts the whole WebKit root, which includes cookies, localStorage and IndexedDB")
	}
	if !strings.Contains(dirs, "NetworkCache") {
		t.Error("the cache list no longer names NetworkCache, so nothing would ever be reclaimed")
	}
}

func TestMacOSHandlesTerminationSignals(t *testing.T) {
	src := readProjectFile(t, "app_darwin.go")
	if !strings.Contains(src, "signal.Notify(") {
		t.Fatal("no signal handler: a logout, restart or shutdown then kills the app with writes in flight and leaves no trace")
	}
	if !strings.Contains(src, "whatsappDeskRequestTerminate") {
		t.Error("the signal handler does not end the run loop through AppKit, so WebKit is never given the chance to close its store in order")
	}
	if !strings.Contains(src, "syscall.SIGTERM") {
		t.Error("SIGTERM is not handled: it is the signal macOS sends a GUI app on logout and shutdown")
	}
}

func TestUpdateWaitsForExitBeforeReplacingTheBundle(t *testing.T) {
	src := readProjectFile(t, "updater_darwin.go")

	// A fixed one-second sleep raced the app's own teardown: the bundle could be
	// moved and replaced while WebKit still had the website data store open.
	if strings.Contains(src, "sleep 1\n") {
		t.Error("the update helper still waits a fixed second instead of waiting for the app to exit, so it can swap the bundle while the session store is still open")
	}
	if !strings.Contains(src, `kill -0 "$app_pid"`) {
		t.Error("the update helper does not wait for the app process to disappear")
	}
	if !strings.Contains(src, "app_pid=") {
		t.Error("the app's pid is not passed to the update helper")
	}
	// os.Exit skips every teardown step, including WebKit closing its store.
	if strings.Contains(src, "os.Exit(0)") {
		t.Error("the updater still exits with os.Exit, which skips WebKit's teardown and can leave IndexedDB writes in flight")
	}
	if !strings.Contains(src, "requestAppTerminate()") {
		t.Error("the updater does not quit through AppKit, so the helper has no clean exit to wait for")
	}
}

// --- #73: WebView2 runtime missing ----------------------------------------

func TestWindowsChecksWebView2RuntimeBeforeCreating(t *testing.T) {
	src := readProjectFile(t, "app_windows.go")

	// Assert the call site, not the identifier: the identifier also appears in
	// the function's own declaration, so a bare contains-check would pass even
	// with the call deleted.
	if !strings.Contains(src, "if !ensureWebView2Runtime() {") {
		t.Fatal("runApp never checks for the WebView2 runtime before creating the engine, so a machine without it dies inside the vendored library with no explanation")
	}
	if !strings.Contains(src, "webviewloader.GetInstalledVersion()") {
		t.Error("the runtime check does not ask the loader for the installed version")
	}
	// The constructor must be reached through the guarded wrapper and nowhere
	// else, so every path to it passes the runtime check and the recover.
	if got := strings.Count(src, "webview2.NewWithOptions(opts)"); got != 1 {
		t.Errorf("the WebView2 constructor is called %d times, want exactly 1 (inside the guarded wrapper): a direct call skips the runtime check and turns a library panic into a silent exit", got)
	}
	if !strings.Contains(src, "recover()") {
		t.Error("no recover guards the constructor, so a panic inside the vendored library still kills the app")
	}
}

// --- #72: account switch reads as a restart -------------------------------

func TestWindowsSwitchKeepsTheWindowInPlace(t *testing.T) {
	src := readProjectFile(t, "app_windows.go")

	// Assert that the carried key reaches the restore, not merely that the
	// identifier exists: a bare contains-check also matches the assignment that
	// sets it, so deleting the read would go unnoticed.
	if !strings.Contains(src, "restoreKey := switchMonitorKey") {
		t.Fatal("the outgoing window's monitor is not carried across a switch, so a switch on a secondary display moves the window to the primary one")
	}
	if !strings.Contains(src, "loadWindowStateForMonitor(userDataDir, restoreKey)") {
		t.Error("the carried monitor key never reaches the window restore, so the window still lands wherever a freshly centred window lands")
	}
	// The teardown must let the page's switch veil paint first.
	if !strings.Contains(src, "switchVeilPaintDelay") {
		t.Error("the teardown is not delayed for the switch veil, so the window vanishes before the transition is visible")
	}
	// Terminate posts a quit to the calling thread's queue, so it has to run on
	// the message loop rather than on a timer goroutine.
	if !strings.Contains(src, "time.AfterFunc(switchVeilPaintDelay") {
		t.Error("the delayed teardown is not scheduled, so the veil delay has no effect")
	}
}

// --- account rail: no dead add control once the cap is reached -------------

// TestAddAccountControlHiddenAtTheAccountCap pins the rail behaviour a user
// reported: with the cap at two accounts the add control was still rendered,
// merely greyed out, so it read as a broken button. It must not be rendered at
// all once both chips are on screen — and it must still exist below the cap, or
// a single-account user could never add a second one.
func TestAddAccountControlHiddenAtTheAccountCap(t *testing.T) {
	src := readProjectFile(t, "main.go")

	guard := "if (accounts.length < maxAccountsUI) {"
	gi := strings.Index(src, guard)
	if gi < 0 {
		t.Fatal("the account rail no longer checks for a free slot before offering the add control")
	}
	create := strings.Index(src, "add.textContent = '+'")
	if create < 0 {
		t.Fatal("the add control disappeared entirely; a single-account user could never add a second account")
	}
	if create < gi {
		t.Fatal("the add control is created outside the free-slot check, so it still shows at the cap")
	}

	// The old treatment kept the button and faded it. If that text comes back,
	// the dead control comes back with it.
	if strings.Contains(src, "'Maximum of ' + maxAccountsUI + ' accounts reached'") {
		t.Error("the add control is rendered again at the cap instead of being hidden")
	}
	if strings.Contains(src, "opacity:' + (atMax ? '.45' : '1')") {
		t.Error("the faded treatment returned; the control should be absent at the cap, not dimmed")
	}
}

// --- updater: never advertise a download that is not there ------------------

// TestCheckForUpdateConfirmsTheAssetBeforeOfferingIt guards the report that the
// app offered an update whose download URL answered HTTP 404. The primary path
// learns only the version from the release redirect, so the URL it builds is a
// naming convention rather than a fact. It has to be confirmed before the user
// is handed it, and a confirmed-missing asset has to be reported as such
// instead of silently becoming "up to date".
func TestCheckForUpdateConfirmsTheAssetBeforeOfferingIt(t *testing.T) {
	src := readProjectFile(t, "updater.go")

	probe := strings.Index(src, "if updateAssetMissing(assetURL) {")
	if probe < 0 {
		t.Fatal("checkForUpdate no longer confirms the asset exists, so it can offer a download that 404s")
	}
	offer := strings.Index(src, "info.DownloadURL = assetURL")
	if offer < 0 {
		t.Fatal("the primary update path no longer sets a download URL; the redirect path is dead")
	}
	if probe > offer {
		t.Fatal("the asset is advertised before it is confirmed, so a missing build is still offered")
	}
	if !strings.Contains(src, "info.NoAssetForPlatform = true") {
		t.Error("a missing asset is not reported to the UI, so the user is told they are up to date while a newer release exists")
	}

	// The UI has to be able to tell "no build for you" apart from "up to date",
	// otherwise the new field changes nothing the user can see.
	ui := readProjectFile(t, "main.go")
	if !strings.Contains(ui, "res.no_asset_for_platform") {
		t.Error("the UI never reads no_asset_for_platform, so a release with no build for this platform still reads as up to date")
	}
}

// --- diagnostics: a bounded log and an honest RSS figure -------------------

// TestDebugLogRotatesWhenItGrows pins the fix for a log that grew without limit.
// Users are asked to send wa_debug.log when they report a bug, so it grows
// exactly on the machines that most need it small, and it was opened for append
// with no cap at all.
func TestDebugLogRotatesWhenItGrows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wa_debug.log")

	// A log that is still small must be left alone: rotating it would throw away
	// the very session being diagnosed.
	small := []byte("small log\n")
	if err := os.WriteFile(path, small, 0o644); err != nil {
		t.Fatal(err)
	}
	rotateDebugLog(path)
	if _, err := os.Stat(path + ".1"); err == nil {
		t.Error("a small log was rotated away, losing the session under diagnosis")
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, small) {
		t.Error("a small log was modified")
	}

	// A log over the cap must move aside, and the previous generation must be
	// replaced rather than accumulated.
	big := bytes.Repeat([]byte("x"), debugLogMaxBytes+1)
	if err := os.WriteFile(path, big, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".1", []byte("previous generation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rotateDebugLog(path)

	if _, err := os.Stat(path); err == nil {
		t.Error("the oversized log is still in place, so the next open appends to a file already over the cap")
	}
	got, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("the oversized log was not kept as the previous generation: %v", err)
	}
	if !bytes.Equal(got, big) {
		t.Error("the rotated log does not hold the file that was just rotated out")
	}
}

// TestProcessRSSDoesNotComeFromTheGoHeap pins the fix for a diagnostic that
// reported the wrong number. The rss= figure was the heap Go's runtime had
// reserved rather than the memory the OS had given the process, so every
// measurement in the log was wrong by an order of magnitude — which is why the
// renderer's real growth had to be measured by hand instead of read from the
// log the app already writes.
func TestProcessRSSDoesNotComeFromTheGoHeap(t *testing.T) {
	// Comments are stripped before scanning: the doc comments name the old
	// mistake, and a guard its own explanation can trip is worse than none.
	src := stripCommentLines(readProjectFile(t, "debuglog.go"))
	if strings.Contains(src, "MemStats") {
		t.Error("the diagnostics log is reporting Go's reserved heap again instead of the process's resident size")
	}
	if !strings.Contains(src, "readProcessRSSKB()") {
		t.Error("the resident size is not read from the operating system")
	}
	if !strings.Contains(src, `cacheDebugLog("stats phase=%s rss=%s source=%s`) {
		t.Error("the stats line does not name where the number came from, so a wrong figure cannot be spotted in the log")
	}

	// The reader itself must work on the platform being diagnosed, or the log
	// stays silent about memory exactly where it matters.
	kb, source, ok := readProcessRSSKB()
	if !ok {
		t.Skipf("no OS resident-size source here (%s); the log will report unavailable", source)
	}
	if source == "unavailable" {
		t.Error("a successful read reported the unavailable source")
	}
	if kb < 512 {
		t.Errorf("resident size %dKB is implausibly small; it looks like a page count or a zero rather than kilobytes", kb)
	}
	t.Logf("resident size: %dMB (source %s)", kb/1024, source)
}

// --- Windows tray: fail-open, and never swallow the app's own close --------

// TestWindowsTrayIsFailOpenAndDoesNotSwallowTheAppsOwnClose guards the tray
// added for Windows, which had none while macOS and Linux both did. This code
// cannot be exercised by the suite — it only compiles on Windows — so the two
// properties that make it safe to ship are pinned here instead:
//
//  1. if the tray cannot be created the window is left completely alone, so the
//     worst case is exactly the behaviour that shipped before; and
//  2. the app's own WM_CLOSE is never intercepted, because that is how runApp
//     tears its window down to rebuild it on an account switch — swallowing it
//     would leave the old window alive beside the new one.
func TestWindowsTrayIsFailOpenAndDoesNotSwallowTheAppsOwnClose(t *testing.T) {
	tray := stripCommentLines(readProjectFile(t, "tray_windows.go"))

	create := strings.Index(tray, "if !trayCreate() {")
	if create < 0 {
		t.Fatal("trayAttach no longer checks whether the tray could be created")
	}
	subclass := strings.Index(tray, "traySubclassMainWindow(hwnd)")
	if subclass < 0 {
		t.Fatal("the close button is never routed through the tray, so hiding the window has no effect")
	}
	if create > subclass {
		t.Error("the window is subclassed before the tray is known to exist, so a failed tray leaves a window that can be hidden with no way back")
	}

	// Only the user's close request may be intercepted. WM_CLOSE is the app's
	// own teardown path.
	if strings.Contains(tray, "trayWmClose") {
		t.Error("the tray intercepts WM_CLOSE, which is how the app tears its own window down on an account switch")
	}
	if !strings.Contains(tray, "if msg == trayWmSysCommand && wparam&0xfff0 == trayScClose {") {
		t.Error("the user's close request (WM_SYSCOMMAND/SC_CLOSE) is not intercepted, so the close button still ends the app")
	}

	// The Win32 callback address is handed to the OS, so the Go value has to
	// stay reachable or the collector may free it while Windows still calls it.
	if !strings.Contains(tray, "= syscall.NewCallback(trayWindowProc)") {
		t.Error("the tray window procedure is not held in a package variable; the collector may free it while Windows still calls it")
	}
	if !strings.Contains(tray, "= syscall.NewCallback(trayMainWindowProc)") {
		t.Error("the subclassed window procedure is not held in a package variable; the collector may free it while Windows still calls it")
	}

	app := readProjectFile(t, "app_windows.go")
	if !strings.Contains(app, "trayAttach(hwnd, func() { w.Terminate() }") {
		t.Error("runApp never attaches the tray, so Windows still has no tray icon")
	}
	if !strings.Contains(app, "trayDetach()") {
		t.Error("the tray icon is never removed on exit, leaving a ghost icon in the notification area")
	}
}
