package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// releaseVersionFromSource reads the version SSOT out of updater.go.
func releaseVersionFromSource(t *testing.T) string {
	t.Helper()
	src := readProjectFile(t, "updater.go")
	re := regexp.MustCompile(`(?m)^var appVersion = "([^"]+)"$`)
	m := re.FindStringSubmatch(src)
	if m == nil {
		t.Fatal("updater.go no longer declares appVersion as a plain string literal")
	}
	return m[1]
}

// TestReleaseVersionMatchesChangelogHead pins the release checklist step that
// is easiest to forget: bumping appVersion in updater.go without adding the
// matching CHANGELOG section (or the reverse). A release whose notes describe
// a different version than the binary reports is worse than no notes.
func TestReleaseVersionMatchesChangelogHead(t *testing.T) {
	version := releaseVersionFromSource(t)
	changelog := readProjectFile(t, "CHANGELOG.md")

	head := regexp.MustCompile(`(?m)^## \[([^\]]+)\]`)
	m := head.FindStringSubmatch(changelog)
	if m == nil {
		t.Fatal("CHANGELOG.md has no version sections")
	}
	if m[1] != version {
		t.Fatalf("updater.go says %q but the newest CHANGELOG section is [%s]; the release notes and the binary would disagree", version, m[1])
	}
}

// TestBuildScriptsDeriveVersionFromTheSSOT guards the defect that shipped with
// the 1.6.6 release: build_mac.sh, build_windows.sh and
// build_windows_installer.sh all defaulted VERSION to a hardcoded "1.6.4", so
// calling them bare stamped a stale appVersion into the binary (-X
// main.appVersion=1.6.4) and the in-app updater then believed the app was
// outdated forever — an update banner that never goes away because the
// "updated" binary is still 1.6.4. The scripts must derive their default from
// updater.go, the declared version SSOT.
func TestBuildScriptsDeriveVersionFromTheSSOT(t *testing.T) {
	for _, script := range []string{"build_mac.sh", "build_windows.sh", "build_windows_installer.sh"} {
		src := readProjectFile(t, script)
		if strings.Contains(src, "${1:-1.6.") {
			t.Errorf("%s still hardcodes a default version; it must derive VERSION from appVersion in updater.go", script)
		}
		if !strings.Contains(src, `sed -n 's/^var appVersion = "\(.*\)"$/\1/p' updater.go`) {
			t.Errorf("%s no longer derives its default VERSION from updater.go; a bare call would stamp a stale version again", script)
		}
	}
}

// TestOnboardingTitleShipsTheRealVersion pins the second stale-copy defect
// found with it: the welcome overlay greeted new users with "WhatsApp Desk
// 1.6.4 is ready" on every later release. The title must go through the
// __WA_APP_VERSION__ placeholder that getOnboardingScript replaces with
// appVersion — the same convention the other injected scripts follow.
func TestOnboardingTitleShipsTheRealVersion(t *testing.T) {
	src := readProjectFile(t, "onboarding.go")

	if strings.Contains(src, "is ready</h1>") && !strings.Contains(src, "WhatsApp Desk __WA_APP_VERSION__ is ready</h1>") {
		t.Error("the onboarding title hardcodes a version again instead of using __WA_APP_VERSION__")
	}
	if !strings.Contains(src, `strings.ReplaceAll(`) || !strings.Contains(src, `"__WA_APP_VERSION__", appVersion`) {
		t.Error("getOnboardingScript no longer replaces __WA_APP_VERSION__ with appVersion; the welcome screen would show a literal placeholder")
	}
}

// TestUpdaterAssetNamesExistInTheChecksumList verifies statically (this host
// has no Windows to test on) that every asset name the updater can resolve for
// darwin and windows is exactly a name make_checksums.sh would have listed in
// SHA256SUMS — the updater verifies its download against that file, so a
// naming drift between updater.go and the release pipeline would fail every
// future self-update.
func TestUpdaterAssetNamesExistInTheChecksumList(t *testing.T) {
	names := map[string]bool{
		"WhatsApp-Desk-macOS-Universal.zip":  false,
		"WhatsApp-macOS-Universal.zip":       false,
		"WhatsAppDesk.exe":                   false,
		"WhatsApp-Desk-macOS-Universal.dmg":  false,
		"WhatsApp-Desk-Windows-x64-Setup.exe": false,
	}
	data, err := os.ReadFile("SHA256SUMS")
	if err != nil {
		t.Skipf("SHA256SUMS not present in the working tree: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 {
			if _, ok := names[fields[1]]; ok {
				names[fields[1]] = true
			}
		}
	}
	for name, seen := range names {
		if !seen {
			t.Errorf("%s is referenced by the updater or release scripts but absent from SHA256SUMS; self-update verification would break", name)
		}
	}
}
