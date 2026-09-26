package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetUniqueFilePath(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "wa_test_unique")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	f1 := getUniqueFilePath(tempDir, "cat.png")
	if filepath.Base(f1) != "cat.png" {
		t.Errorf("expected cat.png, got %s", filepath.Base(f1))
	}
	_ = os.WriteFile(f1, []byte("cat1"), 0644)

	f2 := getUniqueFilePath(tempDir, "cat.png")
	if filepath.Base(f2) != "cat (1).png" {
		t.Errorf("expected cat (1).png, got %s", filepath.Base(f2))
	}
	_ = os.WriteFile(f2, []byte("cat2"), 0644)

	f3 := getUniqueFilePath(tempDir, "cat.png")
	if filepath.Base(f3) != "cat (2).png" {
		t.Errorf("expected cat (2).png, got %s", filepath.Base(f3))
	}
}

func TestSaveDownloadedFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "wa_test_download")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	testContent := "WhatsApp Desktop Light Media Test"
	b64 := "data:text/plain;base64," + base64.StdEncoding.EncodeToString([]byte(testContent))

	savedPath, err := saveDownloadedFileToDir(tempDir, "sample_notes.txt", b64)
	if err != nil {
		t.Fatalf("saveDownloadedFileToDir failed: %v", err)
	}

	if !strings.HasPrefix(savedPath, tempDir) {
		t.Errorf("expected file saved in tempDir %s, got %s", tempDir, savedPath)
	}

	content, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatalf("failed reading saved file: %v", err)
	}
	if string(content) != testContent {
		t.Errorf("content mismatch: got %q, want %q", string(content), testContent)
	}
}

func TestSaveDownloadedFileReusesIdenticalDownload(t *testing.T) {
	tempDir := t.TempDir()
	content := []byte("same WhatsApp attachment")
	b64 := "data:application/octet-stream;base64," + base64.StdEncoding.EncodeToString(content)

	first, err := saveDownloadedFileToDir(tempDir, "document.pdf", b64)
	if err != nil {
		t.Fatal(err)
	}
	second, err := saveDownloadedFileToDir(tempDir, "document.pdf", b64)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("identical download created a duplicate: first=%q second=%q", first, second)
	}
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("identical download created %d files, want 1", len(entries))
	}
}

func TestValidateDownloadDirRejectsSensitive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	blocked := []string{
		"/proc",
		"/proc/self",
		"/sys",
		"/etc",
		"/etc/cron.d",
		// NOTE: plain /root is intentionally usable (see
		// TestValidateDownloadDirRootUser); only autostart/systemd below a
		// home dir are blocked.
		filepath.Join(home, ".config", "autostart"),
		filepath.Join(home, ".config", "autostart", "2026-01"),
		filepath.Join(home, ".local", "share", "applications"),
		filepath.Join(home, ".config", "systemd", "user"),
	}
	for _, dir := range blocked {
		if err := validateDownloadDir(dir); err == nil {
			t.Errorf("validateDownloadDir(%q) = nil, want error", dir)
		}
	}

	// A symlink pointing at a blocked location must be rejected too.
	link := filepath.Join(home, "my-downloads")
	if err := os.Symlink(filepath.Join(home, ".config", "autostart"), link); err != nil {
		t.Fatal(err)
	}
	if err := validateDownloadDir(link); err == nil {
		t.Errorf("validateDownloadDir(symlink %q -> autostart) = nil, want error", link)
	}

	if err := validateDownloadDir("   "); err == nil {
		t.Error("validateDownloadDir(empty) = nil, want error")
	}
}

func TestValidateDownloadDirAcceptsNormal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	ok := []string{
		filepath.Join(home, "Downloads", "WhatsApp Downloads"),
		filepath.Join(home, "Downloads", "WhatsApp Downloads", "2026-01"), // monthly subfolder
		t.TempDir(),
	}
	for _, dir := range ok {
		if err := validateDownloadDir(dir); err != nil {
			t.Errorf("validateDownloadDir(%q) = %v, want nil", dir, err)
		}
	}
}

func TestValidateDownloadDirRootUser(t *testing.T) {
	// Running as root keeps HOME=/root, so the default download folder must
	// stay usable even though system prefixes are blocked. Autostart under
	// /root must still be rejected.
	t.Setenv("HOME", "/root")
	t.Setenv("XDG_CONFIG_HOME", "/root/.config")

	if err := validateDownloadDir("/root/Downloads/WhatsApp Downloads"); err != nil {
		t.Errorf("validateDownloadDir(root default) = %v, want nil", err)
	}
	if err := validateDownloadDir("/root/.config/autostart"); err == nil {
		t.Error("validateDownloadDir(/root/.config/autostart) = nil, want error")
	}
}

func TestSaveDownloadedFileRefusesSensitiveDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b64 := "data:text/plain;base64," + base64.StdEncoding.EncodeToString([]byte("pwn"))
	if _, err := saveDownloadedFileToDir(filepath.Join(home, ".config", "autostart"), "x.txt", b64); err == nil {
		t.Fatal("saveDownloadedFileToDir(autostart) = nil, want error")
	}
	// Nothing must have been created there.
	if _, err := os.Stat(filepath.Join(home, ".config", "autostart", "x.txt")); !os.IsNotExist(err) {
		t.Fatal("sensitive directory was populated despite validation")
	}
}

func TestOpenFileIsJailedToDownloadAndPreviewDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	dl := filepath.Join(home, "Downloads", "WA")
	if err := os.MkdirAll(dl, 0755); err != nil {
		t.Fatal(err)
	}
	// Point settings at our temp download dir via env-isolated config home.
	s := loadSettings()
	s.DownloadDir = dl
	if err := saveSettings(s); err != nil {
		t.Fatal(err)
	}

	inside := filepath.Join(dl, "2026-01", "doc.pdf")
	if err := os.MkdirAll(filepath.Dir(inside), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inside, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if !isAllowedOpenPath(inside) {
		t.Errorf("isAllowedOpenPath(%q) = false, want true (inside download dir)", inside)
	}

	for _, bad := range []string{"/etc/passwd", "/etc/hosts", filepath.Join(home, ".bashrc")} {
		if isAllowedOpenPath(bad) {
			t.Errorf("isAllowedOpenPath(%q) = true, want false", bad)
		}
	}
	if openFileInDefaultApp("/etc/passwd") {
		t.Error("openFileInDefaultApp(/etc/passwd) = true, want false")
	}
}
func TestSaveDownloadedFileKeepsDifferentContent(t *testing.T) {
	tempDir := t.TempDir()
	encode := func(value string) string {
		return "data:text/plain;base64," + base64.StdEncoding.EncodeToString([]byte(value))
	}

	first, err := saveDownloadedFileToDir(tempDir, "report.txt", encode("first"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := saveDownloadedFileToDir(tempDir, "report.txt", encode("second"))
	if err != nil {
		t.Fatal(err)
	}
	if second == first || filepath.Base(second) != "report (1).txt" {
		t.Fatalf("different content must be preserved separately, got %q", second)
	}
}

func TestSaveDownloadedFileRejectsOversizedPayload(t *testing.T) {
	prev := maxAttachmentBytes
	maxAttachmentBytes = 1024
	t.Cleanup(func() { maxAttachmentBytes = prev })

	dir := t.TempDir()
	big := "data:application/octet-stream;base64," + base64.StdEncoding.EncodeToString(make([]byte, 2048))
	if _, err := saveDownloadedFileToDir(dir, "big.bin", big); err == nil {
		t.Fatal("oversized attachment must be rejected before decode")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatal("rejected attachment must leave nothing on disk")
	}
}
