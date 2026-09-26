package main

import (
	"encoding/json"
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
		"activeDownloadKeys[requestKey] = { status: 'downloading' }",
		"markDownloadComplete(requestKey, savedPath)",
		"releaseDownloadRequest(requestKey)",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("download de-duplication is missing %q", want)
		}
	}
}

func TestExistingDownloadsAreDeduplicatedByContent(t *testing.T) {
	script := getInitScript("test-agent")
	for _, want := range []string{
		"window.saveDownloadedFileNative(filename, base64data)",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("content-based download reuse is missing %q", want)
		}
	}
	if strings.Contains(script, "window.findDownloadedFileNative(filename)") {
		t.Fatal("downloads must not be reused by filename before their content is verified")
	}
}

func TestDuplicateDownloadToastReportsAlreadySaved(t *testing.T) {
	script := getInitScript("test-agent")
	for _, want := range []string{
		"var savedDownloadPaths = Object.create(null)",
		"var alreadySaved = savedDownloadPaths[savedPath] === true",
		"savedDownloadPaths[savedPath] = true",
		"'💾 File already saved: ' + filename",
		"'📄 Already saved: ' + filename",
		"'💾 Saved successfully: ' + filename",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("duplicate download feedback is missing %q", want)
		}
	}
}

func TestFilePickerUploadsDoNotTriggerDocumentPreview(t *testing.T) {
	script := getInitScript("test-agent")
	for _, want := range []string{
		"function handleFileInputChange(e)",
		"input.type !== 'file'",
		"if (input.files && input.files.length > 0) lastUploadAt = Date.now();",
		"document.addEventListener('change', handleFileInputChange, true)",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("file-picker upload guard is missing %q", want)
		}
	}
}

func TestExplicitDocumentDownloadsDoNotAutoOpenPreview(t *testing.T) {
	script := getInitScript("test-agent")
	if strings.Count(script, "captureDownload(href, name, false)") < 2 {
		t.Fatal("explicit document download paths must disable auto-preview")
	}
	for _, want := range []string{
		"var lastExplicitDownloadAt = 0",
		"function isRecentExplicitDownload()",
		"!isRecentExplicitDownload()",
		"function isExplicitDownloadMenuItem(target)",
		"target.closest('[role=\"menuitem\"]')",
		"target.closest(viewerDownloadSelector)",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("explicit download preview guard is missing %q", want)
		}
	}
}

func TestSavedBadgeFollowsFilenameElement(t *testing.T) {
	script := getInitScript("test-agent")
	for _, want := range []string{
		"function findFileNameElement(el)",
		"function decorateItem(el, name, filenameEl)",
		"var host = filenameEl || el.querySelector",
		"if (name) decorateItem(row, name, findFileNameElement(row))",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("saved badge alignment is missing %q", want)
		}
	}
}

func TestDownloadFilenameResolutionAvoidsGenericNames(t *testing.T) {
	script := getInitScript("test-agent")
	for _, want := range []string{
		"function cleanDownloadFilename(name)",
		"function isPlaceholderDownloadFilename(name)",
		"function filenameFromContentDisposition(header)",
		"function resolveDownloadFilename(filename, contentDisposition)",
		"Content-Disposition",
		"whatsapp_file",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("download filename handling is missing %q", want)
		}
	}
	if strings.Contains(script, "var name = downloadAttr || this.download || lastClickedDocName || 'whatsapp_media'") {
		t.Fatal("download anchors must resolve generic filenames before saving")
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

	// SheetJS is no longer inlined into the page script (that would force the page to
	// parse ~430 KB on every load, even when no spreadsheet is ever opened). It is
	// embedded in the binary and pulled in on demand through the native bridge, so the
	// script must reference the lazy loader and its consumers.
	for _, want := range []string{
		"function ensureXLSXLoaded()",
		"loadXLSXLibraryNative",
		"XLSX.read(rawXlsxB64",
		"XLSX.utils.sheet_to_html",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("spreadsheet preview script is missing %q", want)
		}
	}

	// xlsxLibJS is exactly what the native binding hands to the page, so the embedded
	// asset must be a real, complete SheetJS core build, not an empty placeholder.
	if len(xlsxLibJS) < 100000 {
		t.Fatalf("embedded SheetJS asset looks truncated (%d bytes)", len(xlsxLibJS))
	}
	for _, want := range []string{"make_xlsx_lib", "sheet_to_html"} {
		if !strings.Contains(xlsxLibJS, want) {
			t.Errorf("embedded SheetJS asset is missing %q", want)
		}
	}
}

// Every platform must expose the lazy SheetJS bridge; a missing binding would
// silently disable spreadsheet preview on that OS alone.
func TestAllPlatformsExposeXLSXBridge(t *testing.T) {
	for _, file := range []string{"app_darwin.go", "app_windows.go", "app_linux.go"} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		content := string(source)
		if !strings.Contains(content, `Bind("loadXLSXLibraryNative"`) {
			t.Errorf("%s does not bind loadXLSXLibraryNative", file)
		}
		if !strings.Contains(content, "return xlsxLibJS") {
			t.Errorf("%s does not return the embedded SheetJS library", file)
		}
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

func TestBackgroundEnhancementsYieldDuringScrolling(t *testing.T) {
	script := getInitScript("test-agent")
	for _, want := range []string{
		"waBackgroundWorkBusyUntil", "markBackgroundWorkBusy",
		"window.addEventListener('wheel', markBackgroundWorkBusy", "Date.now() < waBackgroundWorkBusyUntil",
		"pendingMediaRoots.length >= 24", "pendingSpellRoots.length < 12",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("performance guard is missing %q", want)
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
		"wa-btn-run-diagnostics", "wa-btn-show-shortcuts",
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

func TestSettingsShortcutActionsKeepConsistentSpacing(t *testing.T) {
	script := getInitScript("test-agent")
	for _, shortcut := range []string{"+Shift+P", "+Shift+T", "+Shift+M", "+Shift+S"} {
		idx := strings.Index(script, shortcut)
		if idx < 0 {
			t.Fatalf("settings shortcut %q not found", shortcut)
		}
		window := script[idx-500 : idx+500]
		if !strings.Contains(window, "gap:12px;min-width:150px;flex-shrink:0") {
			t.Errorf("shortcut %q is missing the spaced action layout", shortcut)
		}
		if !strings.Contains(window, "min-width:78px") {
			t.Errorf("shortcut %q action button is missing a consistent minimum width", shortcut)
		}
	}
}

func TestSettingsAlwaysHasAnAccessibleEntryPoint(t *testing.T) {
	script := getInitScript("test-agent")
	start := strings.Index(script, "function injectHeaderToolbarBtn()")
	end := strings.Index(script, "window.showSettingsModal = function()")
	if start < 0 || end < 0 || end <= start {
		t.Fatal("settings entry-point implementation is incomplete")
	}
	entryPoints := script[start:end]
	if strings.Contains(entryPoints, "function injectHeaderToolbarBtn() {\n\t\t\t\tif (shouldPauseBackgroundWork())") {
		t.Fatal("essential Settings button must not be deferred by scroll throttling")
	}
	for _, want := range []string{
		"wa-toolbar-settings-btn",
		"wa-settings-fallback-btn",
		"ensureSettingsFallback",
		"isElementVisible",
		"header.lastElementChild || header",
		"setTimeout(ensureSettingsEntryPoints, 600)",
		"window.addEventListener('keydown', function(e)",
		"}, true);",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("settings must remain reachable on changing WhatsApp UI; missing %q", want)
		}
	}
}

func TestPrivacyModeUsesVisualBlurWithChatListHoverUnblur(t *testing.T) {
	script := getInitScript("test-agent")
	for _, want := range []string{
		"filter: blur(6px) !important",
		"filter: none !important",
		"[data-testid=\"msg-container\"]:hover",
		"data-wa-privacy-hover",
		"data-wa-privacy-reveal",
		"function privacyChatRowFromTarget(target)",
		"function isPrivacyArchivedInfo(target)",
		"if (!isPrivacyArchivedInfo(revealed[i])",
		"if (isPrivacyArchivedInfo(revealTargets[i])) continue",
		"function markPrivacyHoverRow(row)",
		"function updatePrivacyHoverFromTarget(target)",
		"data-wa-privacy-chat-row",
		"data-wa-privacy-avatar",
		"function markPrivacyAvatarTargets(row, avatarSelector)",
		"WhatsApp renders initials as text inside a circular slot",
		"Clear stale reveal markers globally",
		"function markPrivacyChatRows()",
		"function markArchivedPrivacyViews()",
		"function scheduleArchivedPrivacyMark()",
		"[0, 100, 300]",
		"!row.matches('[data-testid=\"cell-frame-container\"], div._ak8l')",
		"setProperty('filter', 'none', 'important')",
		"div._ak8l",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("privacy blur styling is missing %q", want)
		}
	}
	if strings.Contains(script, "background: rgba(134,150,160") {
		t.Fatal("privacy mode must use visual blur, not gray solid redaction boxes")
	}
	if strings.Contains(script, "#pane-side span:hover") {
		t.Fatal("chat-list privacy reveal must remain scoped to the hovered chat container")
	}
	if strings.Contains(script, "#pane-side [role=\"row\"]:hover span") {
		t.Fatal("chat-list privacy reveal must not depend on broad row hover selectors")
	}
	if !strings.Contains(script, "[data-wa-privacy-chat-row=\"1\"] span,") {
		t.Fatal("chat-list timestamps must be included in the marked chat-row blur layer")
	}
	if !strings.Contains(script, "row.querySelectorAll('span, ._ak8q") {
		t.Fatal("hover reveal must include timestamp spans")
	}
}

func TestPrivacyModeCopyMatchesTimestampBlurBehavior(t *testing.T) {
	script := getInitScript("test-agent")
	if !strings.Contains(script, "Hide names, previews, timestamps & message text") {
		t.Fatal("privacy mode copy must explain that timestamps are hidden")
	}
	if strings.Contains(script, "timestamps stay visible") {
		t.Fatal("privacy mode copy must not claim timestamps stay visible")
	}
}

func TestPrivacyModeCoversStickersAndRevealsOnlyTheirMessage(t *testing.T) {
	script := getInitScript("test-agent")
	checks := []string{
		`[data-testid="sticker-container"]`,
		`[data-testid="animated-sticker"]`,
		`img[src*=".webp"][data-testid*="sticker" i]`,
		`[data-testid="msg-container"]:hover [data-testid="sticker-container"]`,
		`.message-in:hover [data-testid="sticker-container"]`,
		`.message-out:hover [data-testid="sticker-container"]`,
		`[role="row"]:hover [data-testid="sticker-container"]`,
	}
	for _, want := range checks {
		if !strings.Contains(script, want) {
			t.Errorf("privacy sticker handling is missing %q", want)
		}
	}
}

func TestPrivacyModeCoversDocumentPreviewsAndQuotedMedia(t *testing.T) {
	script := getInitScript("test-agent")
	checks := []string{
		`[role="row"] img:not([data-emoji])`,
		`[role="row"] canvas`,
		`[role="row"] iframe`,
		`[role="row"] [style*="background-image"]`,
		`[data-testid="quoted-message"]`,
		`[role="row"]:hover img`,
		`[role="row"]:hover [data-testid="quoted-message"]`,
	}
	for _, want := range checks {
		if !strings.Contains(script, want) {
			t.Errorf("privacy attachment/reply handling is missing %q", want)
		}
	}
}

func TestPrivacyModeCoversArchivedChatsAndAllAvatarVariants(t *testing.T) {
	script := getInitScript("test-agent")
	checks := []string{
		// Sidebar and archived rows are marked only when they contain chat text and an avatar.
		"[data-wa-privacy-chat-row=\"1\"] span",
		"[data-wa-privacy-archived-row=\"1\"] span",
		"var avatarSelector = 'img, image, ._ak8h",
		"[data-testid*=\"avatar\" i]",
		"svg[viewBox=\"0 0 49 49\"]'",
		"[data-wa-privacy-avatar=\"1\"]",
		"data-wa-privacy-archived-info",
		"function markArchivedInfo(view)",
		"candidate.style.setProperty('filter', 'none', 'important')",
		"replace(/\\s+/g, ' ')",
		"PRIVACY_ARCHIVED_LABEL_RE",
		"PRIVACY_ARCHIVED_INFO_RE",
		"privacyIsArchivedNavigationText",
		"privacyIsArchivedInfoText",
		"function privacyLooksLikeArchivedView(anchor)",
		"[data-icon=\"default-user\"]",
		"These chats stay archived when new messages are received",
		"function isArchivedChatRow(row)",
		"function isPrivacySidebarControl(node)",
		"data-wa-privacy-archive-control",
		"control.removeAttribute('data-wa-privacy-chat-row')",
		"var archivedLabels = document.querySelectorAll('#side span, #pane-side span, #side [role=\"button\"], #pane-side [role=\"button\"]')",
		"labelParent.clientHeight >= 40",
		"function forceArchivedControlVisible()",
		"control.getBoundingClientRect",
		"control.style.setProperty('filter', 'none', 'important')",
		"var archiveIcons = document.querySelectorAll('[data-icon*=\"archive\" i]",
		"icon.style.setProperty('filter', 'none', 'important')",
		"new MutationObserver(function()",
		"observePrivacySidebar()",
		"function schedulePrivacySidebarRefresh()",
		"clearTimeout(privacySidebarRefreshTimer)",
		"data-wa-privacy-archived-info-checked",
		".privacy-mode.blur-avatars #main header ._ak8h",
		// Sparing timestamps in #side (including archived view)
		"tagTimesIn(document.getElementById('side') || document.getElementById('pane-side'))",
	}
	for _, want := range checks {
		if !strings.Contains(script, want) {
			t.Errorf("privacy rules missing required coverage for %q", want)
		}
	}
	if strings.Contains(script, "[role=\"button\"] > div:first-child") {
		t.Fatal("avatar privacy must not blur generic button child containers")
	}
	if strings.Contains(script, "var avatarSelector = 'img, image, ._ak8h, [data-testid=\"default-user\"], [data-testid*=\"avatar\" i], [data-icon=\"default-user\"], [data-icon=\"default-group\"], svg[viewBox=\"0 0 49 49\"], [style*=\"background-image\"]") {
		t.Fatal("avatar privacy must not use a broad background-image selector")
	}
	if strings.Contains(script, "div.x78zum5 > div.x6s0dn4 > div") {
		t.Fatal("avatar privacy must not blur unstable layout containers")
	}
	if strings.Contains(script, ".privacy-mode.blur-avatars #side [role=\"row\"]:hover img") {
		t.Fatal("sidebar avatar reveal must not depend on broad parent hover selectors")
	}
	if strings.Contains(script, "markArchivedInfo(document.body)") {
		t.Fatal("archived guidance must not scan the whole document")
	}
	if strings.Contains(script, "#side span, #side div, #pane-side span, #pane-side div") {
		t.Fatal("archived privacy must not repeatedly scan every sidebar div")
	}
}

func TestThemeReapplyIsBoundedAndAvoidsObserverFeedbackLoop(t *testing.T) {
	script := getInitScript("test-agent")
	for _, want := range []string{
		"data-wa-desk-theme",
		"function scheduleThemeReapply()",
		"[0, 350, 1200, 2600]",
		"wa-desk-theme-style",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("stable theme application is missing %q", want)
		}
	}
	if strings.Contains(script, "themeObserver") {
		t.Fatal("theme class observer can enter a WhatsApp feedback loop")
	}
}

func TestSettingsHelpUsesLocalDiagnosticsAndDocumentsShortcuts(t *testing.T) {
	script := getInitScript("test-agent")
	for _, want := range []string{
		"Help & diagnostics", "Run quick check", "View keyboard shortcuts",
		"Native bridge: ready", "Local settings storage: ready",
		"getDownloadDirNative", "checkForUpdateNative", "isMac ? 'Cmd' : 'Ctrl'",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("settings help is missing %q", want)
		}
	}
}

func TestUpdateProgressKeepsStatusTextInSync(t *testing.T) {
	script := getInitScript("test-agent")
	start := strings.Index(script, "window.onUpdateProgress = function(pct)")
	end := strings.Index(script[start:], "window.onUpdateStatus = function(statusMsg)")
	if start < 0 || end < 0 {
		t.Fatal("update progress handlers not found")
	}
	progress := script[start : start+end]
	for _, want := range []string{
		"wa-update-progress-bar",
		"wa-update-progress-pct",
		"wa-update-text",
		"Downloading update package... ' + pct + '%'",
	} {
		if !strings.Contains(progress, want) {
			t.Errorf("update progress handler is missing %q", want)
		}
	}
}

func TestUpdateBannerReservesLayoutSpace(t *testing.T) {
	script := getInitScript("test-agent")
	for _, want := range []string{
		"html.wa-update-visible #app",
		"--wa-update-banner-height",
		"banner.offsetHeight",
		"layoutRoot.classList.add('wa-update-visible')",
		"bannerResizeObserver.disconnect()",
		"document.documentElement.classList.remove('wa-update-visible')",
		"document.documentElement.style.removeProperty('--wa-update-banner-height')",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("update banner layout compensation is missing %q", want)
		}
	}
}

func TestUpdaterErrorKeepsClearMessageAndRetryAction(t *testing.T) {
	script := getInitScript("test-agent")
	start := strings.Index(script, "window.onUpdateError = function(errMsg)")
	end := strings.Index(script[start:], "// Manual Check Function")
	if start < 0 || end < 0 {
		t.Fatal("updater error handler not found")
	}
	errorHandler := script[start : start+end]
	for _, want := range []string{
		"Update failed: ",
		"retry.textContent = 'Retry'",
		"actions.style.display = 'flex'",
		"prog.style.display = 'none'",
	} {
		if !strings.Contains(errorHandler, want) {
			t.Errorf("updater recovery handler is missing %q", want)
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

func TestMacMediaPermissionUXIncludesSettingsAndRetryControls(t *testing.T) {
	script := getInitScript("test-agent")
	for _, want := range []string{
		"getCameraPermissionNative",
		"getMicrophonePermissionNative",
		"openMediaPrivacySettingsNative('camera')",
		"wa-media-permission-settings",
		"wa-media-permission-retry",
		"Privacy & Security → Camera/Microphone",
		"getUserMedia({ audio: true, video: true })",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("macOS media permission UX is missing %q", want)
		}
	}
}

func TestNotificationToggleGatesNativeNotificationsAcrossPlatforms(t *testing.T) {
	script := getInitScript("test-agent")
	for _, want := range []string{
		"getNotificationsEnabledNative",
		"setNotificationsEnabledNative",
		"window.isNotificationsEnabled",
		"window.setNotificationsEnabled",
		"if (notificationsStateReady && notificationsEnabled && window.sendNativeNotification)",
		"Desktop Notifications",
		"wa-action-toggle-notifications",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("cross-platform notification toggle is missing %q", want)
		}
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
	script := getInitScript("test-agent")
	start := strings.Index(script, "window.closeDocumentViewerAfterNativePreview = function()")
	end := strings.Index(script[start:], "// Handle explicit user clicks on WhatsApp Web's Media Viewer")
	if start < 0 || end < 0 {
		t.Fatal("native preview close handler boundaries not found")
	}
	handler := script[start : start+end]
	if !strings.Contains(handler, "document.querySelector('[data-testid=\"media-viewer\"]')") {
		t.Fatal("native preview close handler must scope dismissal to WhatsApp's media viewer")
	}
	if strings.Contains(handler, "document.dispatchEvent(esc)") || strings.Contains(handler, "window.dispatchEvent(esc)") {
		t.Fatal("native preview close handler must not dispatch a global Escape")
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

func TestWindowsCacheProbeNeverShowsTerminal(t *testing.T) {
	source, err := os.ReadFile("cache_budget_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	content := string(source)
	for _, want := range []string{
		`powershell.exe`,
		`"-NonInteractive"`,
		`"-WindowStyle", "Hidden"`,
		`syscall.SysProcAttr{HideWindow: true}`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("Windows cache probe must prevent console flash; missing %q", want)
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

func TestEscapeChatPreservesFullscreen(t *testing.T) {
	script := getInitScript("test-agent")
	for _, want := range []string{
		"waRunModule('escape-chat'",
		"if (__WA_GOOS !== 'darwin') return;",
		"nativeOverlayOpen()",
		"activeChatHeader()",
		"button[data-testid=\"back\"]",
		"e.preventDefault()",
		"e.stopImmediatePropagation()",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("Escape chat safeguard is missing %q", want)
		}
	}
}

func TestDragAndDropUploadStabilization(t *testing.T) {
	script := getInitScript("test-agent")

	checks := []string{
		"isRecentUpload()",
		"setFilesOnInput(fileInput, files)",
		"findAttachButton()",
		"findInputInOrNear(el)",
		"findMediaInput()",
		"findDocumentInput()",
		"injectFiles(files, attempt",
		"document.addEventListener('dragenter'",
		"document.addEventListener('dragleave'",
		"document.addEventListener('dragover'",
		"document.addEventListener('drop'",
		"DataTransfer()",
		"fileInput.dispatchEvent(new Event('input'",
		"fileInput.dispatchEvent(new Event('change'",
		"stopImmediatePropagation",
		"areAllMediaFiles(files)",
	}

	for _, want := range checks {
		if !strings.Contains(script, want) {
			t.Errorf("script is missing drag-and-drop safeguard %q", want)
		}
	}
}

// TestDragOverStateClearedOnEveryDrop guards against a stuck .wa-drag-over
// class. That class sets `pointer-events: none` on every element, so if the
// drop handler returns early (drop over an excluded target such as a dialog,
// or a drop whose file list is empty — cloud placeholder files on Windows)
// without resetting first, every click in the app goes dead until reload:
// typing and Enter keep working, which is exactly why it surfaces as "the
// @mention popup appears but contacts cannot be selected". The reset must run
// before any early return, and safety nets must exist for drags that end
// without a matching dragleave.
func TestDragOverStateClearedOnEveryDrop(t *testing.T) {
	script := getInitScript("test-agent")

	// A dedicated reset helper must exist and must be the first statement of
	// handleDrop, ahead of every early return in that function.
	if !strings.Contains(script, "function clearDragVisualState() {") {
		t.Fatal("script is missing the clearDragVisualState helper")
	}
	dropIdx := strings.Index(script, "function handleDrop(e) {")
	if dropIdx == -1 {
		t.Fatal("script is missing the handleDrop function")
	}
	body := script[dropIdx : dropIdx+600]
	if !strings.Contains(body, "clearDragVisualState();") {
		t.Error("handleDrop does not call clearDragVisualState() before its early returns")
	}
	if strings.Contains(body, "dragCounter = 0") {
		t.Error("handleDrop still resets drag state inline instead of via clearDragVisualState")
	}

	// Safety nets for drags that end without a matching dragleave.
	for _, want := range []string{
		"document.addEventListener('dragend', clearDragVisualState",
		"window.addEventListener('blur', clearDragVisualState)",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script is missing drag-state safety net %q", want)
		}
	}
}

func TestSettingsFallbackButtonsAreHarmonized(t *testing.T) {
	script := getInitScript("test-agent")

	// Both fallback buttons must use bottom:14px;left:14px; to avoid vertical shifting
	if !strings.Contains(script, "id = 'wa-settings-fallback-btn'") {
		t.Fatal("missing wa-settings-fallback-btn")
	}
	if !strings.Contains(script, "id = 'wa-emergency-settings-btn'") {
		t.Fatal("missing wa-emergency-settings-btn")
	}

	for _, want := range []string{
		"left:14px;bottom:14px;z-index:9999998;width:38px;height:38px",
		"left:14px;bottom:14px;z-index:2147483646;width:38px;height:38px",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("expected button positioning %q in script", want)
		}
	}
}

func TestToastNotificationHasHighestZIndex(t *testing.T) {
	script := getInitScript("test-agent")

	// Toast popup must have maximum z-index (2147483647) to stay in front of settings overlay (9999999)
	want := "id = 'wa-hud-toast';\n\t\t\t\ttoast.style.cssText = 'position:fixed;top:16px;left:50%;transform:translateX(-50%);background:rgba(32,44,51,0.94);backdrop-filter:blur(10px);color:#00a884;border:1px solid rgba(0,168,132,0.4);border-radius:20px;padding:8px 20px;font-size:12.5px;font-weight:600;z-index:2147483647;"
	if !strings.Contains(script, "z-index:2147483647") {
		t.Errorf("toast notification must use z-index 2147483647 to prevent being hidden behind modal, got: %s", want)
	}
}

func TestAutoStartStateRefreshesFromNative(t *testing.T) {
	script := getInitScript("test-agent")

	for _, want := range []string{
		"window.getAutoStartNative",
		"refreshAutoStartState",
		"window.refreshAutoStartState",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("auto-start module missing %q", want)
		}
	}
}

func TestWindowStateMaximizedSerialization(t *testing.T) {
	s := WindowState{
		X:         100,
		Y:         200,
		Width:     1200,
		Height:    800,
		Maximized: true,
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("failed to marshal WindowState: %v", err)
	}
	if !strings.Contains(string(data), `"maximized":true`) {
		t.Errorf("expected maximized in JSON, got: %s", string(data))
	}

	var parsed WindowState
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal WindowState: %v", err)
	}
	if !parsed.Maximized {
		t.Errorf("expected parsed.Maximized to be true")
	}
}

func TestDocModalEscapesAttackerControlledStrings(t *testing.T) {
	script := getInitScript("test-agent")

	if !strings.Contains(script, "function escapeHtml(s) {") {
		t.Fatal("injected script is missing the escapeHtml helper")
	}
	// Attacker-controlled values (chat-supplied filenames, saved paths,
	// release titles) must never be concatenated raw into innerHTML.
	for _, raw := range []string{
		`title="' + filename + '">' + filename + '</strong>'`,
		`break-all;">' + filename + '</h2>'`,
		`">' + displayPath + '</div>'`,
		`title="' + filename + '"></iframe>'`,
		`color:#e9edef;">' + titleText + '</strong>'`,
	} {
		if strings.Contains(script, raw) {
			t.Errorf("unescaped attacker-controlled interpolation remains: %q", raw)
		}
	}
	for _, want := range []string{
		`escapeHtml(filename) + '">' + escapeHtml(filename)`,
		`escapeHtml(filename) + '</h2>'`,
		`escapeHtml(displayPath)`,
		`escapeHtml(filename) + '"></iframe>'`,
		`escapeHtml(titleText)`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("escaped interpolation missing: %q", want)
		}
	}
}

func TestEscapeHtmlNeutralizesMarkup(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not available")
	}
	script := getInitScript("test-agent")
	start := strings.Index(script, "function escapeHtml(s) {")
	if start < 0 {
		t.Fatal("escapeHtml helper not found")
	}
	end := strings.Index(script[start:], "\n\t\t}\n")
	if end < 0 {
		t.Fatal("escapeHtml helper is incomplete")
	}
	fn := script[start : start+end+len("\n\t\t}")]

	probe := fn + `
	if (escapeHtml('"><img src=x onerror=alert(1)>.pdf') !== '&quot;&gt;&lt;img src=x onerror=alert(1)&gt;.pdf') throw new Error('attr/text breakout not escaped: ' + escapeHtml('"><img src=x onerror=alert(1)>.pdf'));
	if (escapeHtml("a'b&c") !== 'a&#39;b&amp;c') throw new Error('quote/ampersand not escaped');
	if (escapeHtml('laporan akhir.pdf') !== 'laporan akhir.pdf') throw new Error('valid filename altered');
	if (escapeHtml('') !== '' || escapeHtml(null) !== '' || escapeHtml(undefined) !== '') throw new Error('empty/nullish mishandled');
	console.log('escapeHtml OK');
	`
	cmd := exec.Command("node", "-e", probe)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("escapeHtml probe failed: %v\n%s", err, output)
	}
}

// TestSpreadsheetPreviewSanitizesCellMarkup guards the spreadsheet preview.
// The table comes from XLSX.utils.sheet_to_html, which escapes cell *text* but
// writes the raw value into a data-v attribute: a cell whose value contains a
// double quote closes that attribute and injects live markup, reachable from
// any spreadsheet sent in a chat. The generated HTML must therefore never be
// assigned to innerHTML unsanitized.
func TestSpreadsheetPreviewSanitizesCellMarkup(t *testing.T) {
	script := getInitScript("test-agent")

	if !strings.Contains(script, "function sanitizeSheetHtml(html) {") {
		t.Fatal("injected script is missing the sanitizeSheetHtml helper")
	}
	if !strings.Contains(script, "sanitizeSheetHtml(XLSX.utils.sheet_to_html(") {
		t.Error("sheet_to_html output is not passed through sanitizeSheetHtml")
	}
	if strings.Contains(script, "var tableHtml = XLSX.utils.sheet_to_html(") {
		t.Error("raw sheet_to_html output is still assigned to tableHtml")
	}
}
