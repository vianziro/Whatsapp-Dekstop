package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDarwinUpdaterSelectsOnlyMacArchive(t *testing.T) {
	release := &GitHubRelease{Assets: []GitHubAsset{
		{Name: "WhatsApp-Desk-Windows-x64.zip", BrowserDownloadURL: "https://example.test/windows.zip"},
		{Name: "WhatsApp-Desk-macOS-Universal.zip", BrowserDownloadURL: "https://example.test/macos.zip"},
	}}
	asset := findAssetForOS(release, "darwin")
	if asset == nil || asset.Name != "WhatsApp-Desk-macOS-Universal.zip" {
		t.Fatalf("macOS self-update must select the macOS archive, got %#v", asset)
	}
}

func TestLinuxUpdaterSelectsOnlyLinuxArchive(t *testing.T) {
	release := &GitHubRelease{Assets: []GitHubAsset{
		{Name: "unrelated.tar.gz", BrowserDownloadURL: "https://example.test/unrelated.tar.gz"},
		{Name: "WhatsApp-Desk-Linux-x64.tar.gz", BrowserDownloadURL: "https://example.test/linux.tar.gz"},
	}}
	asset := findAssetForOS(release, "linux")
	if asset == nil || asset.Name != "WhatsApp-Desk-Linux-x64.tar.gz" {
		t.Fatalf("Linux self-update must select the Linux archive, got %#v", asset)
	}
}

func TestLinuxUpdaterSelectsPortableArchive(t *testing.T) {
	release := &GitHubRelease{Assets: []GitHubAsset{
		{Name: "WhatsApp-Desk-Linux-amd64.deb", BrowserDownloadURL: "https://example.test/app.deb"},
		{Name: "WhatsApp-Desk-Linux-x64.tar.gz", BrowserDownloadURL: "https://example.test/app.tar.gz"},
	}}
	asset := findAssetForOS(release, "linux")
	if asset == nil || asset.Name != "WhatsApp-Desk-Linux-x64.tar.gz" {
		t.Fatalf("Linux self-update must select tar.gz, got %#v", asset)
	}
	if got := updateDownloadExtension(asset.BrowserDownloadURL); got != ".tar.gz" {
		t.Fatalf("download extension = %q, want .tar.gz", got)
	}
}

func TestLinuxArm64UpdaterNeverFallsBackToX64(t *testing.T) {
	release := &GitHubRelease{Assets: []GitHubAsset{
		{Name: "WhatsApp-Desk-Linux-x64.tar.gz", BrowserDownloadURL: "https://example.test/x64.tar.gz"},
		{Name: "WhatsApp-Desk-Linux-arm64.tar.gz", BrowserDownloadURL: "https://example.test/arm64.tar.gz"},
	}}
	asset := findAssetForPlatform(release, "linux", "arm64")
	if asset == nil || asset.Name != "WhatsApp-Desk-Linux-arm64.tar.gz" {
		t.Fatalf("Linux arm64 updater must select arm64 archive, got %#v", asset)
	}

	missing := &GitHubRelease{Assets: []GitHubAsset{
		{Name: "WhatsApp-Desk-Linux-x64.tar.gz", BrowserDownloadURL: "https://example.test/x64.tar.gz"},
	}}
	if asset := findAssetForPlatform(missing, "linux", "arm64"); asset != nil {
		t.Fatalf("Linux arm64 updater must not download x64 fallback, got %#v", asset)
	}
	if got := updateAssetForPlatform("linux", "arm64"); !strings.HasSuffix(got, "Linux-arm64.tar.gz") {
		t.Fatalf("Linux arm64 stable asset URL = %q", got)
	}
}

func TestFindLinuxAssetPrefersWebkit41WhenRequested(t *testing.T) {
	release := &GitHubRelease{Assets: []GitHubAsset{
		{Name: "WhatsApp-Desk-Linux-x64.tar.gz", BrowserDownloadURL: "https://example.test/x64.tar.gz"},
		{Name: "WhatsApp-Desk-Linux-x64-webkit4.1.tar.gz", BrowserDownloadURL: "https://example.test/x64-41.tar.gz"},
	}}
	if asset := findLinuxAsset(release, "x64", true); asset == nil || asset.Name != "WhatsApp-Desk-Linux-x64-webkit4.1.tar.gz" {
		t.Fatalf("preferWebkit41 must select the -webkit4.1 tarball, got %#v", asset)
	}
	if asset := findLinuxAsset(release, "x64", false); asset == nil || asset.Name != "WhatsApp-Desk-Linux-x64.tar.gz" {
		t.Fatalf("without preference must keep the historical tarball, got %#v", asset)
	}
}

func TestFindLinuxAssetFallsBackToPlain(t *testing.T) {
	// Releases predating the 4.1 variant carry only the plain tarball; the
	// preference must never turn into a "no update" on those releases.
	release := &GitHubRelease{Assets: []GitHubAsset{
		{Name: "WhatsApp-Desk-Linux-x64.tar.gz", BrowserDownloadURL: "https://example.test/x64.tar.gz"},
	}}
	if asset := findLinuxAsset(release, "x64", true); asset == nil || asset.Name != "WhatsApp-Desk-Linux-x64.tar.gz" {
		t.Fatalf("missing -webkit4.1 asset must fall back to plain, got %#v", asset)
	}
}

func TestFindLinuxAssetNeverFallsBackCrossArch(t *testing.T) {
	release := &GitHubRelease{Assets: []GitHubAsset{
		{Name: "WhatsApp-Desk-Linux-x64-webkit4.1.tar.gz", BrowserDownloadURL: "https://example.test/x64-41.tar.gz"},
	}}
	if asset := findLinuxAsset(release, "arm64", true); asset != nil {
		t.Fatalf("arm64 must never receive an x64 tarball, got %#v", asset)
	}
}

func TestWebkitGTK40DetectorAgreesWithLdconfig(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("detector only probes on Linux")
	}
	if linuxLdconfigPath() == "" {
		t.Skip("ldconfig not found")
	}
	out, err := exec.Command(linuxLdconfigPath(), "-p").Output()
	if err != nil {
		t.Skipf("ldconfig -p failed: %v", err)
	}
	want := strings.Contains(string(out), webkit40LibPrefix)
	if got := webkitGTK40Present(); got != want {
		t.Fatalf("webkitGTK40Present() = %v, ldconfig says %v", got, want)
	}
}

func TestLinuxLdconfigPathIsAbsolute(t *testing.T) {
	// A GUI-launched app may have a PATH without /sbin, so the probe must
	// resolve an absolute path rather than rely on PATH alone.
	got := linuxLdconfigPath()
	if got == "" {
		t.Skip("ldconfig not installed on this host")
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("ldconfig path %q must be absolute", got)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("resolved ldconfig %q is not usable: %v", got, err)
	}
}

func TestLinuxLdconfigPathSurvivesEmptyPath(t *testing.T) {
	// Regression: with a desktop-launcher PATH that omits /sbin and
	// /usr/sbin, resolving ldconfig through PATH alone used to fail, and the
	// fail-open fallback then hid the 4.1-only case this probe exists for.
	if runtime.GOOS != "linux" {
		t.Skip("ldconfig probe only applies to Linux")
	}
	t.Setenv("PATH", t.TempDir())
	got := linuxLdconfigPath()
	if got == "" {
		if _, err := exec.LookPath("ldconfig"); err == nil {
			t.Skip("ldconfig resolvable even without PATH lookup")
		}
		t.Skip("ldconfig not installed on this host")
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("ldconfig path %q must be absolute even with an empty PATH", got)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("resolved ldconfig %q is not usable: %v", got, err)
	}
}

func TestWebkit40InDirsFindsLibrary(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, webkit40LibPrefix+"0"), nil, 0644); err != nil {
		t.Fatalf("seed fake library: %v", err)
	}
	found, anyReadable := webkit40InDirs([]string{dir})
	if !found || !anyReadable {
		t.Fatalf("webkit40InDirs() = (%v, %v), want (true, true)", found, anyReadable)
	}
}

func TestWebkit40InDirsProvesAbsence(t *testing.T) {
	// A readable directory without the library is evidence of absence — this
	// is what lets the detector stop failing open on 4.1-only systems.
	found, anyReadable := webkit40InDirs([]string{t.TempDir()})
	if found {
		t.Fatalf("webkit40InDirs() found a library in an empty directory")
	}
	if !anyReadable {
		t.Fatalf("a readable directory must report anyReadable = true")
	}
}

func TestWebkit40InDirsNoReadableDirIsInconclusive(t *testing.T) {
	found, anyReadable := webkit40InDirs([]string{
		filepath.Join(t.TempDir(), "does-not-exist"),
	})
	if found || anyReadable {
		t.Fatalf("webkit40InDirs() = (%v, %v), want (false, false)", found, anyReadable)
	}
}

func TestWebkit40SearchDirsCoverUsrMergeAndFedora(t *testing.T) {
	dirs := strings.Join(webkit40SearchDirs(), " ")
	for _, want := range []string{
		"/usr/lib/x86_64-linux-gnu",
		"/usr/lib/aarch64-linux-gnu",
		"/usr/lib64",
	} {
		if !strings.Contains(dirs, want) {
			t.Errorf("search dirs must include %s, got %s", want, dirs)
		}
	}
}

func TestLinuxStableAssetURLMatchesLocalWebkit(t *testing.T) {
	got := updateAssetForPlatform("linux", "amd64")
	wantVariant := runtime.GOOS == "linux" && !webkitGTK40Present()
	if wantVariant && !strings.HasSuffix(got, "Linux-x64-webkit4.1.tar.gz") {
		t.Fatalf("4.1-only host must get the -webkit4.1 URL, got %q", got)
	}
	if !wantVariant && !strings.HasSuffix(got, "Linux-x64.tar.gz") {
		t.Fatalf("default host must keep the historical URL, got %q", got)
	}
	if !isAllowedUpdateURL(got) {
		t.Fatalf("stable Linux URL %q must pass the updater allowlist", got)
	}
}

func TestWindowsUpdaterSelectsWhatsAppDesk(t *testing.T) {
	release := &GitHubRelease{Assets: []GitHubAsset{
		{Name: "WhatsApp-Desk-Windows-x64.zip", BrowserDownloadURL: "https://example.test/app.zip"},
		{Name: "legacy-helper.exe", BrowserDownloadURL: "https://example.test/legacy.exe"},
		{Name: "WhatsAppDesk.exe", BrowserDownloadURL: "https://example.test/WhatsAppDesk.exe"},
	}}
	asset := findAssetForOS(release, "windows")
	if asset == nil || asset.Name != "WhatsAppDesk.exe" {
		t.Fatalf("Windows self-update must select WhatsAppDesk.exe, got %#v", asset)
	}
	if got := updateDownloadExtension(asset.BrowserDownloadURL); got != ".exe" {
		t.Fatalf("download extension = %q, want .exe", got)
	}
}

func TestWindowsUpdaterRecoversWhenExecutableReplacementFails(t *testing.T) {
	batch := windowsUpdateBatch(4321, `C:\Temp\update.exe`, `C:\Apps\WhatsAppDesk.exe`)
	for _, want := range []string{
		`:copy_loop`,
		`if not errorlevel 1 goto restart`,
		`goto copy_failed`,
		`:copy_failed`,
		`WhatsAppDesk-update.log`,
		`The existing version was restarted.`,
		`:restart`,
	} {
		if !strings.Contains(batch, want) {
			t.Errorf("Windows updater recovery script is missing %q", want)
		}
	}
	if strings.Index(batch, `:copy_failed`) > strings.Index(batch, `:restart`) {
		t.Fatal("failed-copy recovery must be defined before the normal restart path")
	}
}

func TestProgressWriterFallback(t *testing.T) {
	var reported []int
	pw := &progressWriter{
		total: -1, // Unknown or chunked Content-Length
		onProgress: func(pct int) {
			reported = append(reported, pct)
		},
	}

	chunk := make([]byte, 1024*1024) // 1MB
	_, _ = pw.Write(chunk)
	if len(reported) == 0 {
		t.Errorf("expected progress reported on fallback, got empty")
	}
}

func TestIsAllowedUpdateURL(t *testing.T) {
	allowed := []string{
		"https://github.com/vianziro/Whatsapp-Dekstop/releases/latest/download/WhatsAppDesk.exe",
		"https://github.com/vianziro/Whatsapp-Dekstop/releases/latest/download/WhatsApp-Desk-Linux-x64.tar.gz",
		"https://github.com/vianziro/Whatsapp-Dekstop/releases/download/v1.5.9.8/WhatsApp-Desk-macOS-Universal.zip",
		// Query strings must not bypass the check.
		"https://github.com/vianziro/Whatsapp-Dekstop/releases/download/v1.5.9.8/WhatsAppDesk.exe?token=abc",
	}
	for _, u := range allowed {
		if !isAllowedUpdateURL(u) {
			t.Errorf("isAllowedUpdateURL(%q) = false, want true", u)
		}
	}

	blocked := []string{
		"",
		"not a url",
		"http://github.com/vianziro/Whatsapp-Dekstop/releases/latest/download/WhatsAppDesk.exe", // plain HTTP
		"https://evil.example.com/WhatsAppDesk.exe",
		"https://evil.example.com/vianziro/Whatsapp-Dekstop/releases/download/v1/evil.exe", // wrong host
		"https://github.com.evil.example.com/vianziro/Whatsapp-Dekstop/releases/latest/download/x.exe",
		"https://github.com/other-owner/Whatsapp-Dekstop/releases/latest/download/x.exe", // wrong owner
		"https://github.com/vianziro/other-repo/releases/latest/download/x.exe",          // wrong repo
		"https://github.com/vianziro/Whatsapp-Dekstop/blob/main/main.go",                 // not a release
		"https://objects.githubusercontent.com/evil-payload",                             // CDN bypass attempt
		"file:///tmp/evil.exe",
		"javascript:alert(1)",
	}
	for _, u := range blocked {
		if isAllowedUpdateURL(u) {
			t.Errorf("isAllowedUpdateURL(%q) = true, want false", u)
		}
	}
}

func TestIsNewerVersion(t *testing.T) {
	cases := []struct {
		current string
		latest  string
		want    bool
	}{
		{"1.4.0", "1.4.0", false},
		{"1.4.0", "v1.4.0", false},
		{"1.4.0", "1.4.1", true},
		{"1.4.0", "v1.5.0", true},
		{"1.5.0", "v1.5.1", true},
		{"1.5.1", "v1.5.2", true},
		{"1.5.2", "v1.5.3", true},
		{"1.5.9", "1.5.9.1", true},
		{"1.5.9.1", "1.5.9", false},
		{"1.5.9.1", "1.5.9.1", false},
		{"1.5.3", "1.5.3", false},
		{"1.5.2", "1.5.2", false},
		{"1.5.2", "1.5.1", false},
		{"1.5.1", "v1.5.1", false},
		{"1.5.1", "1.5.0", false},
		{"1.4.0", "2.0.0", true},
		{"1.4.0", "1.3.9", false},
		{"1.4.0", "1.4.0-beta", false},
	}

	for _, c := range cases {
		got := isNewerVersion(c.current, c.latest)
		if got != c.want {
			t.Errorf("isNewerVersion(%q, %q) = %v; want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestCheckForUpdateLive(t *testing.T) {
	// Version 1.0.0 should always detect an update on live repo
	infoOld, err := checkForUpdate("1.0.0")
	if err != nil {
		t.Skipf("skipping live network test: %v", err)
		return
	}
	if !infoOld.Available {
		t.Errorf("expected update for 1.0.0, got false")
	}
	if infoOld.LatestVersion == "" {
		t.Errorf("expected non-empty latest version")
	}
	if infoOld.DownloadURL == "" {
		t.Errorf("expected non-empty download URL")
	}
}

func TestDownloadEnforcesSizeLimit(t *testing.T) {
	prev := maxUpdateDownloadBytes
	maxUpdateDownloadBytes = 64 << 10
	t.Cleanup(func() { maxUpdateDownloadBytes = prev })

	// Undeclared length (chunked) with a body far beyond the cap: the
	// limiter must stop the write and remove the partial file.
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := make([]byte, 32<<10)
		for i := 0; i < 8; i++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
			w.(http.Flusher).Flush()
		}
	}))
	defer big.Close()

	dest := filepath.Join(t.TempDir(), "update.bin")
	err := downloadFileWithProgress(big.URL, dest, nil)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized download must be rejected, got %v", err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatal("partial download must be removed after rejection")
	}

	// Honest small body still passes.
	small := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("tiny-payload"))
	}))
	defer small.Close()
	if err := downloadFileWithProgress(small.URL, filepath.Join(t.TempDir(), "ok.bin"), nil); err != nil {
		t.Fatalf("small download must pass: %v", err)
	}

	// Declared Content-Length above the cap fails before any byte is read.
	huge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1073741824")
		w.WriteHeader(http.StatusOK)
	}))
	defer huge.Close()
	if err := downloadFileWithProgress(huge.URL, filepath.Join(t.TempDir(), "huge.bin"), nil); err == nil {
		t.Fatal("declared oversized download must fail fast")
	}
}
