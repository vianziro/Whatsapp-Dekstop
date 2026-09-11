package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestPDFPreviewOwnsBlobURLLifecycle(t *testing.T) {
	script := getInitScript("test-agent")

	checks := []string{
		"origCreateObjectURL(previewBlob)",
		"URL.revokeObjectURL(ownedBlobUrl)",
		"isRecentPDFIntent()",
		"blob.type === 'application/octet-stream'",
		"findDocumentDownloadControl(el)",
		"e.stopImmediatePropagation()",
		"extractDocumentName(el)",
	}
	for _, want := range checks {
		if !strings.Contains(script, want) {
			t.Errorf("PDF preview script is missing %q", want)
		}
	}
}

func TestWebViewDoesNotAdvertiseMissingChromePDFPlugin(t *testing.T) {
	script := getInitScript("test-agent")
	for _, unsupported := range []string{
		"get: () => true,\n\t\t\t\tconfigurable: true\n\t\t\t});\n\n\t\t\tif (!navigator.mimeTypes",
		"Chrome PDF Viewer",
		"internal-pdf-viewer",
	} {
		if strings.Contains(script, unsupported) {
			t.Errorf("WKWebView must not advertise unsupported PDF capability %q", unsupported)
		}
	}
}

func TestPDFDownloadDiscoveryDoesNotDependOnLegacyViewerTestID(t *testing.T) {
	script := getInitScript("test-agent")
	for _, want := range []string{
		"function findVisibleViewerDownloadControl()",
		"document.querySelectorAll(viewerDownloadSelector)",
		"rect.top < window.innerHeight * 0.3",
		"pendingViewerDownloadClick",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("PDF toolbar discovery is missing %q", want)
		}
	}
}

func TestDownloadInterceptorCoalescesDuplicateRequests(t *testing.T) {
	script := getInitScript("test-agent")
	for _, want := range []string{
		"var activeDownloadKeys = Object.create(null)",
		"function downloadRequestKey(href, filename)",
		"var activeDownloadSizes = {}",
		"if (blob.size && activeDownloadSizes[blob.size])",
		"activeDownloadKeys[requestKey] = { status: 'downloading' }",
		"markDownloadComplete(requestKey, savedPath, blob.size)",
		"releaseDownloadRequest(requestKey)",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("download de-duplication is missing %q", want)
		}
	}
}

func TestOfficeDocumentPreviewSupport(t *testing.T) {
	script := getInitScript("test-agent")

	checks := []string{
		"readZipEntryText",
		"parseDocxToHtml",
		"parsePptxToHtml",
		"renderSpreadsheetPreview",
		"XLSX.read(rawXlsxB64",
		"XLSX.utils.sheet_to_html",
		"Open in Excel / Numbers",
		"Open in Word / Pages",
		"Open in PowerPoint / Keynote",
		"isDocumentFileName(foundName)",
	}
	for _, want := range checks {
		if !strings.Contains(script, want) {
			t.Errorf("Office document preview script is missing %q", want)
		}
	}
}

func TestSpreadsheetPreviewSupportsLegacyXLS(t *testing.T) {
	script := getInitScript("test-agent")

	// The spreadsheet branch must handle .xls (legacy binary Excel) in addition to
	// .xlsx and .csv, and must no longer rely on the old hand-rolled OOXML-only
	// parser, which never supported the legacy binary format at all.
	if !strings.Contains(script, "ext === 'csv' || ext === 'xlsx' || ext === 'xls'") {
		t.Errorf("spreadsheet preview branch does not unify csv/xlsx/xls handling")
	}
	if strings.Contains(script, "parseXlsxToHtml") || strings.Contains(script, "parseCsvToHtml") {
		t.Errorf("legacy hand-rolled spreadsheet parser should have been removed in favor of the bundled XLSX library")
	}
	if !strings.Contains(script, "XLSX.version") && !strings.Contains(script, "make_xlsx_lib") {
		t.Errorf("bundled SheetJS library does not appear to be embedded in the init script")
	}
}

func TestHiddenWindowRequestsNativeMemoryRelease(t *testing.T) {
	script := getInitScript("test-agent")
	for _, want := range []string{"visibilitychange", "releaseMemoryNative", "lastClickedDocName = ''"} {
		if !strings.Contains(script, want) {
			t.Errorf("memory lifecycle script is missing %q", want)
		}
	}
}

func TestSettingsControlsRemainWired(t *testing.T) {
	script := getInitScript("test-agent")
	ids := []string{
		"wa-theme-btn-dark", "wa-theme-btn-light", "wa-theme-btn-system",
		"wa-action-toggle-priv", "wa-action-toggle-pin", "wa-action-toggle-mute", "wa-action-toggle-auto",
		"wa-btn-change-folder", "wa-btn-open-folder", "wa-btn-reset-folder",
		"wa-btn-check-updates-modal", "wa-btn-reload-modal", "wa-btn-hardref-modal", "wa-btn-onboard-modal",
	}
	for _, id := range ids {
		if strings.Count(script, `id="`+id+`"`) != 1 {
			t.Errorf("control %s must be rendered exactly once", id)
		}
		if !strings.Contains(script, "getElementById('"+id+"')") {
			t.Errorf("control %s has no event or state binding", id)
		}
	}
}

func TestThemeSwitchDoesNotOverrideNativeMediaQueriesOrLoseUserChoice(t *testing.T) {
	script := getInitScript("test-agent")
	start := strings.Index(script, "// Theme Manager")
	end := strings.Index(script, "// --- In-Flow Header Toolbar Button")
	if start < 0 || end <= start {
		t.Fatal("theme manager block not found")
	}
	theme := script[start:end]

	if strings.Contains(theme, "window.matchMedia = function") {
		t.Fatal("theme manager must not replace the browser's native MediaQueryList implementation")
	}
	for _, want := range []string{
		"var themeChoiceVersion = 0",
		"if (requestVersion !== themeChoiceVersion) return",
		"window.location.reload()",
	} {
		if !strings.Contains(theme, want) {
			t.Errorf("reliable cross-platform theme switching is missing %q", want)
		}
	}
	if strings.Count(theme, "getAppThemeNative()") != 1 {
		t.Fatal("saved theme must be requested once so stale async responses cannot overwrite a user click")
	}
}

func TestAllPlatformsApplySavedThemeToNativeWindow(t *testing.T) {
	cases := []struct {
		file   string
		marker string
	}{
		{"app_darwin.go", "setNativeWindowTheme"},
		{"app_windows.go", "applyNativeThemeWin"},
		{"app_linux.go", "applyNativeThemeLinux"},
	}
	for _, tc := range cases {
		source, err := os.ReadFile(tc.file)
		if err != nil {
			t.Fatal(err)
		}
		content := string(source)
		if strings.Count(content, tc.marker) < 2 {
			t.Errorf("%s must apply saved theme both at startup and after a settings change", tc.file)
		}
	}
}

func TestOnboardingIsQuietAndReplayable(t *testing.T) {
	script := getOnboardingScript()
	for _, unwanted := range []string{"radial-gradient", "backdrop-filter", "wa-feat-card", "Hemat RAM ~90%"} {
		if strings.Contains(script, unwanted) {
			t.Errorf("onboarding still contains noisy pattern %q", unwanted)
		}
	}
	for _, want := range []string{"window.showOnboardingModal", "prefers-reduced-motion", "Skip guide"} {
		if !strings.Contains(script, want) {
			t.Errorf("onboarding is missing %q", want)
		}
	}
}

func TestInjectedJavaScriptParses(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not available")
	}
	cmd := exec.Command("node", "--check", "-")
	cmd.Stdin = strings.NewReader(getInitScript("test-agent"))
	if output, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(err.Error(), "operation not permitted") || strings.Contains(err.Error(), "permission denied") {
			t.Skipf("skipping node execution in sandboxed environment: %v", err)
		}
		t.Fatalf("injected JavaScript does not parse: %v\n%s", err, output)
	}
}

func TestDarwinMenuBridgeUsesStableAppWindow(t *testing.T) {
	source, err := os.ReadFile("app_darwin.go")
	if err != nil {
		t.Fatal(err)
	}
	script := string(source)
	if !strings.Contains(script, "static NSWindow* appWindow(void)") {
		t.Fatal("native menu must resolve the app's stored window")
	}

	start := strings.Index(script, "@implementation MenuBridge")
	if start < 0 {
		t.Fatal("MenuBridge implementation not found")
	}
	endOffset := strings.Index(script[start:], "@end")
	if endOffset < 0 {
		t.Fatal("MenuBridge implementation has no end")
	}
	bridge := script[start : start+endOffset]
	if strings.Contains(bridge, "[NSApp keyWindow] ?: [NSApp mainWindow]") {
		t.Fatal("menu actions must not depend on the transient key/main window")
	}
	for _, action := range []string{
		"menuSettings:", "menuCheckUpdates:", "menuOpenDownloads:",
		"menuTogglePrivacy:", "menuToggleAlwaysOnTop:", "menuToggleMuteAudio:",
		"menuReloadChat:", "menuHardRefresh:", "menuShowApp:",
		"menuSetThemeDark:", "menuSetThemeLight:", "menuSetThemeSystem:",
	} {
		if !strings.Contains(bridge, action) {
			t.Errorf("native menu action %s is missing", action)
		}
	}
}

func TestDarwinPDFUsesNativePDFKitPreview(t *testing.T) {
	source, err := os.ReadFile("app_darwin.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"-framework PDFKit", "#import <PDFKit/PDFKit.h>",
		"showNativePDFPreview", "showPDFPreviewNative",
	} {
		if !strings.Contains(string(source), want) {
			t.Errorf("native PDF preview is missing %q", want)
		}
	}

	script := getInitScript("test-agent")
	previewStart := strings.Index(script, "function showInAppDocModal")
	overlayStart := strings.Index(script[previewStart:], "var overlay = document.createElement('div')")
	nativeStart := strings.Index(script[previewStart:], "window.showPDFPreviewNative(savedPath)")
	if previewStart < 0 || overlayStart < 0 || nativeStart < 0 || nativeStart > overlayStart {
		t.Fatal("macOS PDFKit preview must run before the unsupported WKWebView overlay is created")
	}
	if !strings.Contains(script, "e.key === 'Escape' && e.isTrusted") {
		t.Fatal("synthetic viewer-dismiss Escape events must not close the document preview")
	}
}

func TestClosingNativePDFReturnsToChat(t *testing.T) {
	darwinSource, err := os.ReadFile("app_darwin.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"PDFPreviewWindowDelegate",
		"windowWillClose:",
		"closeDocumentViewerAfterNativePreview",
		"setDelegate:g_pdfPreviewDelegate",
	} {
		if !strings.Contains(string(darwinSource), want) {
			t.Errorf("native PDF close bridge is missing %q", want)
		}
	}
	if !strings.Contains(getInitScript("test-agent"), "window.closeDocumentViewerAfterNativePreview = function()") {
		t.Fatal("webview has no command that returns from the document viewer to chat")
	}
}

func TestClosingNativePDFReleasesRenderedDocument(t *testing.T) {
	source, err := os.ReadFile("app_darwin.go")
	if err != nil {
		t.Fatal(err)
	}
	content := string(source)
	start := strings.Index(content, "- (void)windowWillClose:")
	if start < 0 {
		t.Fatal("PDF preview close handler not found")
	}
	end := strings.Index(content[start:], "\n}")
	if end < 0 {
		t.Fatal("PDF preview close handler is incomplete")
	}
	handler := content[start : start+end]
	for _, want := range []string{
		"[pdfView setDocument:nil]",
		"[window setContentView:nil]",
		"purgeWebKitMemory()",
	} {
		if !strings.Contains(handler, want) {
			t.Errorf("PDF close handler must release rendered memory via %q", want)
		}
	}
}

func TestNativePDFCanReopenAfterContentCleanup(t *testing.T) {
	source, err := os.ReadFile("app_darwin.go")
	if err != nil {
		t.Fatal(err)
	}
	content := string(source)
	for _, want := range []string{
		"NSRect pdfFrame = [[g_pdfPreviewWindow contentView] bounds]",
		"NSIsEmptyRect(pdfFrame)",
		"initWithFrame:pdfFrame",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("reusable native PDF viewer is missing %q", want)
		}
	}
}

func TestBackgroundDOMWorkPausesWhenHidden(t *testing.T) {
	script := getInitScript("test-agent")
	for _, want := range []string{
		"function shouldPauseBackgroundWork()",
		"if (shouldPauseBackgroundWork()) return;",
		"mediaObserver",
		"viewerObserver",
		"injectHeaderToolbarBtn",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("background work pause guard is missing %q", want)
		}
	}
}

func TestWindowsMemoryReleaseDoesNotOrphanSuspendedWebView(t *testing.T) {
	source, err := os.ReadFile("app_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	content := string(source)
	start := strings.Index(content, `_ = w.Bind("releaseMemoryNative"`)
	if start < 0 {
		t.Fatal("Windows memory release binding not found")
	}
	end := strings.Index(content[start:], "\n\t})")
	if end < 0 {
		t.Fatal("Windows memory release binding is incomplete")
	}
	binding := content[start : start+end]
	for _, line := range strings.Split(binding, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "//") && strings.Contains(line, "w.Suspend()") {
			t.Fatal("visibility-triggered memory release must not suspend WebView2 without a matching resume")
		}
	}
}

func TestMacRoutineMemoryPurgeKeepsDiskCache(t *testing.T) {
	source, err := os.ReadFile("app_darwin.go")
	if err != nil {
		t.Fatal(err)
	}
	content := string(source)
	start := strings.Index(content, "static void purgeWebKitMemory(void)")
	if start < 0 {
		t.Fatal("macOS WebKit memory purge routine not found")
	}
	end := strings.Index(content[start:], "\n}")
	if end < 0 {
		t.Fatal("macOS WebKit memory purge routine is incomplete")
	}
	purge := content[start : start+end]
	if strings.Contains(purge, "WKWebsiteDataTypeDiskCache") {
		t.Fatal("routine RAM cleanup must retain the disk cache")
	}
}

func TestMediaObserverScansOnlyAddedSubtrees(t *testing.T) {
	script := getInitScript("test-agent")
	start := strings.Index(script, "function scanForUnpreparedMedia()")
	if start < 0 {
		t.Fatal("media scan routine not found")
	}
	end := strings.Index(script[start:], "var mediaObserver")
	if end < 0 {
		t.Fatal("media scan routine is incomplete")
	}
	scan := script[start : start+end]
	if strings.Contains(scan, "document.body") || strings.Contains(scan, "document.documentElement") {
		t.Fatal("each mutation-frame must not rescan the whole document for media")
	}
	if !strings.Contains(script, "pendingMediaRoots") {
		t.Fatal("media observer must queue only newly-added subtrees")
	}
}

func TestLinuxDoesNotForceContinuousCompositingOrPeriodicGC(t *testing.T) {
	source, err := os.ReadFile("app_linux.go")
	if err != nil {
		t.Fatal(err)
	}
	content := string(source)
	if strings.Contains(content, `Setenv("WEBKIT_FORCE_COMPOSITING_MODE", "1")`) {
		t.Fatal("Linux must let WebKitGTK choose compositing mode")
	}
	if strings.Contains(content, "time.NewTicker(60 * time.Second)") {
		t.Fatal("Linux must not force full Go GC every minute")
	}
	if !strings.Contains(content, `_ = w.Bind("releaseMemoryNative"`) {
		t.Fatal("Linux must release Go memory when the shared visibility lifecycle requests it")
	}
}

func TestDesktopWindowStateUsesResizeEventsNotPolling(t *testing.T) {
	for _, file := range []string{"app_darwin.go", "app_windows.go", "app_linux.go"} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		content := string(source)
		if !strings.Contains(content, `_ = w.Bind("saveWindowStateNative"`) {
			t.Errorf("%s must bind resize-driven window persistence", file)
		}
		if strings.Contains(content, "time.NewTicker(10 * time.Second)") {
			t.Errorf("%s must not poll window state every 10 seconds", file)
		}
	}
}

func TestMediaDoesNotEagerlyBufferEveryAttachment(t *testing.T) {
	script := getInitScript("test-agent")
	if strings.Contains(script, "setAttribute('preload', 'auto')") {
		t.Fatal("chat media must not preload full files before playback")
	}
	if !strings.Contains(script, "setAttribute('preload', 'metadata')") {
		t.Fatal("chat media should load metadata without buffering full files")
	}
}

func TestWindowsProcessProtectionPermitsChildBreakaway(t *testing.T) {
	source, err := os.ReadFile("app_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	content := string(source)

	// Must permit child breakaway so Chromium sandbox job objects don't fail
	if !strings.Contains(content, "JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK") {
		t.Error("Windows Job Object must include JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK")
	}

	// Must not kill own child processes at startup
	if strings.Contains(content, "Stop-Process") {
		t.Error("Windows startup must not invoke PowerShell Stop-Process which kills active webview instances")
	}

	// Must not pass dangerous flags that trigger black screen on Windows 10/11
	for _, dangerous := range []string{"--disable-gpu-shader-disk-cache", "CalculateNativeWinOcclusion", `--js-flags="`} {
		if strings.Contains(content, dangerous) {
			t.Errorf("Windows browser args contain dangerous flag %q known to cause black screen", dangerous)
		}
	}
}

func TestMediaViewerCloseButtonNotIntercepted(t *testing.T) {
	script := getInitScript("test-agent")

	checks := []string{
		`el.closest('[data-testid="media-viewer"]')`,
		`el.closest('#wa-doc-modal-overlay')`,
		`target.closest('[data-testid="media-viewer"]')`,
		`button[data-testid="x-viewer"]`,
		`[data-icon="x-viewer"]`,
		`Escape`,
		`window.dismissStuckViewer()`,
	}

	for _, want := range checks {
		if !strings.Contains(script, want) {
			t.Errorf("script is missing media viewer safeguard %q", want)
		}
	}
}
