//go:build darwin

package main

import (
	"os"
	"strings"
	"testing"
)

func TestMacProfileStoreIsSelectedBeforeWKWebViewCreation(t *testing.T) {
	source, err := os.ReadFile("vendor/github.com/webview/webview_go/libs/webview/include/webview.h")
	if err != nil {
		t.Fatal(err)
	}
	content := string(source)
	profile := strings.Index(content, "WA_DESK_PROFILE_UUID")
	webView := strings.Index(content, "initWithFrame:configuration:")
	if profile < 0 || webView < 0 || profile > webView {
		t.Fatal("macOS profile datastore must be selected before WKWebView creation")
	}
	for _, marker := range []string{"dataStoreForIdentifier:", "setWebsiteDataStore:"} {
		if !strings.Contains(content, marker) {
			t.Errorf("WebKit profile configuration is missing %q", marker)
		}
	}
}

// macOS versions before 14 (this project still supports 13/Ventura) have no
// named-persistent WKWebsiteDataStore API at all. Falling through silently in
// that case previously left every extra account on the SAME shared default
// store as the first account — no isolation, no fresh QR pairing, ever.
func TestMacProfileStoreFallsBackToEphemeralWhenNamedStoreIsUnavailable(t *testing.T) {
	source, err := os.ReadFile("vendor/github.com/webview/webview_go/libs/webview/include/webview.h")
	if err != nil {
		t.Fatal(err)
	}
	content := string(source)
	for _, marker := range []string{"nonPersistentDataStore", "respondsToSelector:"} {
		if !strings.Contains(content, marker) {
			t.Errorf("WebKit profile configuration is missing %q", marker)
		}
	}
}

// Switching accounts must stay inside the same OS process: relaunching the
// executable bounces the Dock, flashes the window, and was reported by users
// as an unwanted "restart". The single WebView still has to be fully torn
// down before the next one is built, so isolation is never compromised by
// two engines running at once — the loop below achieves that in-process.
func TestMacAccountSwitchStaysInProcessAfterFullWebViewTeardown(t *testing.T) {
	source, err := os.ReadFile("app_darwin.go")
	if err != nil {
		t.Fatal(err)
	}
	content := string(source)

	start := strings.Index(content, "func runApp() {")
	if start < 0 {
		t.Fatal("runApp implementation not found")
	}
	body := content[start:]

	for _, marker := range []string{
		`Bind("getAccountsNative"`,
		`Bind("createAccountNative"`,
		`Bind("renameAccountNative"`,
		`Bind("requestAccountSwitchNative"`,
		"w.Terminate()",
		"w.Destroy()",
		"stopUpdateTicker",
		"for {",
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("macOS account lifecycle is missing %q", marker)
		}
	}

	// The old relaunch-the-whole-executable approach must not come back.
	for _, unwanted := range []string{"exec.Command(executable)", "os.Executable()"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("account switch must not relaunch the process, found %q", unwanted)
		}
	}

	// webview.New must only be (re)built after the previous one is destroyed,
	// so exactly one native WebKit engine is ever live at a time.
	newIdx := strings.Index(body, "webview.New(false)")
	runIdx := strings.Index(body, "w.Run()")
	destroyIdx := strings.Index(body, "w.Destroy()")
	if newIdx < 0 || runIdx < 0 || destroyIdx < 0 || !(newIdx < runIdx && runIdx < destroyIdx) {
		t.Fatal("expected webview.New -> w.Run -> w.Destroy ordering within the switch loop")
	}

	// The update-ticker goroutine must stop before the WebView it targets is
	// destroyed, or a stale w.Dispatch/w.Eval could fire on a dead engine.
	stopIdx := strings.Index(body, "close(stopUpdateTicker)")
	if stopIdx < 0 || stopIdx > destroyIdx {
		t.Fatal("update ticker must be stopped before the WebView is destroyed")
	}
}
