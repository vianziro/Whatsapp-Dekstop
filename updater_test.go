package main

import "testing"

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
