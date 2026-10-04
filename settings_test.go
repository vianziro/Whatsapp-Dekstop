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

// TestIdleMemoryReloadDefaultsOnAndPersists pins the three behaviours the
// renderer-memory work depends on: the setting ships enabled (an opt-in default
// would leave the ~2 GB renderer untouched for everyone who never opens
// Settings), an existing settings.json without the key still gets the default
// rather than silently reading as "off", and an explicit opt-out survives a
// save/load round trip.
func TestIdleMemoryReloadDefaultsOnAndPersists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	if !getIdleMemoryReload() {
		t.Fatal("idle memory reload must default to on")
	}

	// A settings.json written before the feature existed has no
	// idle_memory_reload key; it must not be read as an opt-out.
	path := getSettingsFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"theme":"dark","notifications_enabled":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if !getIdleMemoryReload() {
		t.Fatal("a settings file without the key must fall back to on, not off")
	}

	if setIdleMemoryReload(false) {
		t.Fatal("setIdleMemoryReload(false) must report the stored value")
	}
	if getIdleMemoryReload() {
		t.Fatal("the opt-out must survive a reload from disk")
	}
	if !setIdleMemoryReload(true) || !getIdleMemoryReload() {
		t.Fatal("the setting must round-trip back to on")
	}
}

// TestSanitizeDownloadFilenameMakesNamesWindowsSafe covers #68. The attachment
// name arrives from the chat and used to reach os.OpenFile with only the path
// separators removed, so a document called "Rapat 12:30.pdf" could never be
// saved on Windows — ERROR_INVALID_NAME, reported to the user as a bare
// "Failed to save file.".
func TestSanitizeDownloadFilenameMakesNamesWindowsSafe(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"colon becomes underscore", "Rapat 12:30.pdf", "Rapat 12_30.pdf"},
		{"every illegal character", `a<b>c:d"e|f?g*h.pdf`, "a_b_c_d_e_f_g_h.pdf"},
		{"control characters dropped", "in\x00voice\x1f.pdf", "invoice.pdf"},
		{"trailing dot trimmed", "laporan..", "laporan"},
		{"trailing space trimmed", "laporan ", "laporan"},
		{"reserved device name", "CON.txt", "_CON.txt"},
		{"reserved device name, bare", "nul", "_nul"},
		{"ordinary name untouched", "Surat Keterangan.pdf", "Surat Keterangan.pdf"},
		{"unicode untouched", "Rapat Bulanan – Q3.pdf", "Rapat Bulanan – Q3.pdf"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeDownloadFilename(tc.in)
			if got != tc.want {
				t.Errorf("sanitizeDownloadFilename(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	// Degenerate inputs must still yield something creatable rather than "".
	for _, in := range []string{"", ".", "..", ":", "...", "   "} {
		if got := sanitizeDownloadFilename(in); got == "" || got == "." || got == ".." {
			t.Errorf("sanitizeDownloadFilename(%q) = %q, want a usable name", in, got)
		}
	}

	// A very long name must be truncated but keep its extension, and must never
	// exceed the 255-unit limit a single NTFS component allows.
	long := sanitizeDownloadFilename(strings.Repeat("a", 400) + ".pdf")
	if len([]rune(long)) > 200 {
		t.Errorf("long name not truncated: %d runes", len([]rune(long)))
	}
	if !strings.HasSuffix(long, ".pdf") {
		t.Errorf("truncation dropped the extension: %q", long)
	}
}

// TestSaveDownloadedFileAcceptsWindowsHostileName proves the sanitizer is wired
// into the save path and not merely available: a name that previously failed
// outright must now land on disk under the cleaned name.
func TestSaveDownloadedFileAcceptsWindowsHostileName(t *testing.T) {
	dir := t.TempDir()
	b64 := "data:application/pdf;base64," + base64.StdEncoding.EncodeToString([]byte("%PDF-1.4"))

	path, err := saveDownloadedFileToDir(dir, "Rapat 12:30.pdf", b64)
	if err != nil {
		t.Fatalf("save with a colon in the name failed: %v", err)
	}
	if filepath.Base(path) != "Rapat 12_30.pdf" {
		t.Errorf("saved as %q, want Rapat 12_30.pdf", filepath.Base(path))
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not on disk: %v", err)
	}
}

// TestSaveDownloadedFileFallsBackWhenConfiguredDirIsUnusable covers the other
// half of #68: a download folder that cannot be created (deleted, on an
// unmounted drive, or refused by Windows' Controlled Folder Access) used to end
// the save. It must now fall back to the default folder, and must say so — the
// file landing somewhere other than the folder the user chose is not something
// to do silently.
func TestSaveDownloadedFileFallsBackWhenConfiguredDirIsUnusable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	// A regular file where the download folder's parent should be: MkdirAll
	// can never succeed below it, on any platform.
	blocker := filepath.Join(home, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := loadSettings()
	s.DownloadDir = filepath.Join(blocker, "sub")
	if err := saveSettings(s); err != nil {
		t.Fatal(err)
	}

	b64 := "data:text/plain;base64," + base64.StdEncoding.EncodeToString([]byte("fallback"))

	path, err := saveDownloadedFile("catatan.txt", b64)
	if err != nil {
		t.Fatalf("saveDownloadedFile should have fallen back, got error: %v", err)
	}
	wantDir := filepath.Join(home, "Downloads", "WhatsApp Downloads")
	if filepath.Dir(path) != wantDir {
		t.Errorf("fell back to %q, want %q", filepath.Dir(path), wantDir)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("fallback file not on disk: %v", err)
	}
	if issue := getLastDownloadIssue(); issue == "" {
		t.Error("a fallback save must be reported, not silent")
	}
}

// TestSaveDownloadedFileReportsWhyItFailed pins the diagnosability half: when
// the save genuinely fails, the reason must be readable by the page so the next
// bug report carries it instead of a bare "Failed to save file.".
func TestSaveDownloadedFileReportsWhyItFailed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	// Make the default folder unusable too, so there is nowhere left to try.
	if err := os.WriteFile(filepath.Join(home, "Downloads"), []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := loadSettings()
	s.DownloadDir = getDefaultDownloadDir()
	if err := saveSettings(s); err != nil {
		t.Fatal(err)
	}

	b64 := "data:text/plain;base64," + base64.StdEncoding.EncodeToString([]byte("x"))
	path, err := saveDownloadedFile("catatan.txt", b64)
	if err == nil {
		t.Fatalf("save should have failed, got path %q", path)
	}
	issue := getLastDownloadIssue()
	if issue == "" {
		t.Fatal("a failed save must record why, so the toast can say so")
	}
	if !strings.Contains(issue, "directory") {
		t.Errorf("issue %q does not name the cause", issue)
	}

	// And a later successful save must clear it, or the next toast would blame
	// the wrong file.
	if err := os.Remove(filepath.Join(home, "Downloads")); err != nil {
		t.Fatal(err)
	}
	if _, err := saveDownloadedFile("catatan.txt", b64); err != nil {
		t.Fatalf("save after clearing the blocker failed: %v", err)
	}
	if issue := getLastDownloadIssue(); issue != "" {
		t.Errorf("a clean save must clear the issue, got %q", issue)
	}
}
