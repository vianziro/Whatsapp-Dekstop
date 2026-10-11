package main

import (
	"os"
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
