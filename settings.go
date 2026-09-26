package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var downloadFileMu sync.Mutex

type AppSettings struct {
	DownloadDir          string `json:"download_dir"`
	NotifyOnDownload     bool   `json:"notify_on_download"`
	NotificationsEnabled bool   `json:"notifications_enabled"`
	Theme                string `json:"theme"` // "dark", "light", "system"
	OrganizeByMonth      bool   `json:"organize_by_month"`
	SpellCheckEnabled    bool   `json:"spell_check_enabled"`
	SpellCheckLang       string `json:"spell_check_lang"`
	BlurAvatars          bool   `json:"blur_avatars"`
	// LastCrashNotified is the unix time of the crash log last surfaced to
	// the user via the issue reporter, so the startup nudge fires once.
	LastCrashNotified int64 `json:"last_crash_notified"`
}

func getDefaultDownloadDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, "Downloads", "WhatsApp Downloads")
}

func getSettingsFilePath() string {
	var baseDir string
	if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		baseDir = filepath.Join(home, "Library", "Application Support", "WhatsAppDesk")
	} else {
		configDir, err := os.UserConfigDir()
		if err != nil {
			configDir = os.Getenv("APPDATA")
			if configDir == "" {
				configDir = "."
			}
		}
		baseDir = filepath.Join(configDir, "WhatsAppDesk")
	}
	_ = os.MkdirAll(baseDir, 0755)
	return filepath.Join(baseDir, "settings.json")
}

func loadSettings() *AppSettings {
	s := &AppSettings{
		DownloadDir:          getDefaultDownloadDir(),
		NotifyOnDownload:     true,
		NotificationsEnabled: true,
		Theme:                "dark",
		SpellCheckEnabled:    true,
		SpellCheckLang:       "auto",
	}
	data, err := os.ReadFile(getSettingsFilePath())
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, s)
	if strings.TrimSpace(s.DownloadDir) == "" {
		s.DownloadDir = getDefaultDownloadDir()
	}
	// A tampered or misguided settings.json must never turn the download
	// folder into a persistence primitive (e.g. ~/.config/autostart): fall
	// back to the default instead of writing attacker-controlled bytes there.
	if err := validateDownloadDir(s.DownloadDir); err != nil {
		s.DownloadDir = getDefaultDownloadDir()
	}
	if strings.TrimSpace(s.Theme) == "" {
		s.Theme = "dark"
	}
	if s.SpellCheckLang == "" {
		s.SpellCheckLang = "auto"
	}
	return s
}

func saveSettings(s *AppSettings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(getSettingsFilePath(), data, 0644)
}

func getNotificationsEnabled() bool {
	return loadSettings().NotificationsEnabled
}

func setNotificationsEnabled(enabled bool) bool {
	s := loadSettings()
	s.NotificationsEnabled = enabled
	_ = saveSettings(s)
	return s.NotificationsEnabled
}

func saveTheme(theme string) string {
	if theme != "dark" && theme != "light" && theme != "system" {
		theme = "dark"
	}
	s := loadSettings()
	s.Theme = theme
	_ = saveSettings(s)
	return s.Theme
}

func getUniqueFilePath(dir, filename string) string {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	if base == "" {
		base = "download"
	}
	target := filepath.Join(dir, filename)
	counter := 1
	for {
		if _, err := os.Stat(target); os.IsNotExist(err) {
			return target
		}
		target = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, counter, ext))
		counter++
	}
}

func saveDownloadedFile(filename, dataURI string) (string, error) {
	settings := loadSettings()
	dir := settings.DownloadDir
	// Monthly organization keeps the downloads folder browsable over time;
	// saveDownloadedFileToDir creates the subfolder on demand.
	if settings.OrganizeByMonth {
		dir = filepath.Join(dir, time.Now().Format("2006-01"))
	}
	return saveDownloadedFileToDir(dir, filename, dataURI)
}

// setOrganizeByMonth persists the monthly-organization preference and returns
// the effective value.
func setOrganizeByMonth(on bool) bool {
	s := loadSettings()
	s.OrganizeByMonth = on
	_ = saveSettings(s)
	return s.OrganizeByMonth
}

func getSpellCheckEnabled() bool {
	return loadSettings().SpellCheckEnabled
}

func setSpellCheckEnabled(on bool) bool {
	s := loadSettings()
	s.SpellCheckEnabled = on
	_ = saveSettings(s)
	return s.SpellCheckEnabled
}

func getSpellCheckLang() string {
	return loadSettings().SpellCheckLang
}

func setSpellCheckLang(lang string) string {
	s := loadSettings()
	s.SpellCheckLang = lang
	_ = saveSettings(s)
	return s.SpellCheckLang
}

func getBlurAvatars() bool {
	return loadSettings().BlurAvatars
}

func setBlurAvatars(on bool) bool {
	s := loadSettings()
	s.BlurAvatars = on
	_ = saveSettings(s)
	return s.BlurAvatars
}

// findDownloadedFile reports the first regular file matching filename anywhere
// the saver could have placed it: directly in the downloads folder, or inside
// any YYYY-MM subfolder created by monthly organization. It intentionally does
// not depend on the current preference value so existing files remain usable.
func findDownloadedFile(filename string) string {
	name := filepath.Base(strings.TrimSpace(filename))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return ""
	}
	s := loadSettings()
	dir := s.DownloadDir
	if strings.TrimSpace(dir) == "" {
		return ""
	}
	if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
		return filepath.Join(dir, name)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() || len(e.Name()) != 7 || e.Name()[4] != '-' {
			continue
		}
		candidate := filepath.Join(dir, e.Name(), name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

// fileExistsInDownloadDir reports whether filename exists anywhere the saver
// could have placed it. The badge layer uses this, so it must not depend on
// the current preference value.
func fileExistsInDownloadDir(filename string) bool {
	return findDownloadedFile(filename) != ""
}

// blockedDownloadDirPrefixes lists locations a download folder must never
// point at. The autostart/systemd entries would turn every saved attachment
// into a persistence primitive; the system entries can never be a legitimate
// user download folder and are rejected with a clear error instead of a
// confusing MkdirAll failure.
func blockedDownloadDirPrefixes() []string {
	// NOTE: /root is deliberately NOT blanket-blocked. When the app runs as
	// root, HOME=/root and the default download folder lives under it; the
	// autostart/systemd locations there are still covered by the home-based
	// entries below. A non-root user pointing at /root/... fails on
	// filesystem permissions anyway.
	prefixes := []string{"/dev", "/proc", "/sys", "/etc", "/bin", "/sbin", "/usr", "/boot"}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		home = filepath.Clean(home)
		prefixes = append(prefixes,
			filepath.Join(home, ".config", "autostart"),
			filepath.Join(home, ".local", "share", "applications"),
			filepath.Join(home, ".config", "systemd"),
			filepath.Join(home, ".local", "share", "systemd"),
			filepath.Join(home, ".config", "environment.d"),
		)
	}
	return prefixes
}

// resolveForValidation returns the canonical form of dir, resolving symlinks
// even when the final target does not exist yet. filepath.EvalSymlinks alone
// cannot do that: a dangling link (e.g. ~/my-downloads -> ~/.config/autostart
// before autostart exists) would fail to resolve and slip past the blocklist,
// while MkdirAll later follows the link and creates the sensitive directory.
func resolveForValidation(dir string) string {
	abs := filepath.Clean(dir)
	for i := 0; i < 16; i++ {
		// Longest prefix that exists. Lstat (not Stat) is deliberate: it
		// reports the link itself instead of following it.
		existing := abs
		for {
			if _, err := os.Lstat(existing); err == nil {
				break
			}
			parent := filepath.Dir(existing)
			if parent == existing {
				return abs
			}
			existing = parent
		}
		st, err := os.Lstat(existing)
		if err != nil {
			return abs
		}
		// existing was derived from abs by stripping tail components, so the
		// remainder is "" or starts with a separator.
		rest := strings.TrimPrefix(abs, existing)
		if st.Mode()&os.ModeSymlink == 0 {
			if resolved, err := filepath.EvalSymlinks(existing); err == nil {
				abs = filepath.Clean(filepath.Join(resolved, rest))
			}
			return abs
		}
		target, err := os.Readlink(existing)
		if err != nil {
			return abs
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(existing), target)
		}
		abs = filepath.Clean(filepath.Join(target, rest))
	}
	return abs
}

// validateDownloadDir rejects empty paths, paths escaping to sensitive system
// or autostart locations (including via symlinks, even dangling ones), and
// paths that exist but are not directories. It does not create anything.
func validateDownloadDir(dir string) error {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return fmt.Errorf("download directory is empty")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return fmt.Errorf("invalid download directory: %w", err)
	}
	abs = resolveForValidation(abs)
	for _, blocked := range blockedDownloadDirPrefixes() {
		// Resolve the blocklist entry the same way as the candidate. On macOS
		// /etc and /var are symlinks into /private, so comparing a resolved
		// candidate against an unresolved entry never matches and the whole
		// check silently no-ops (validateDownloadDir("/etc") returned nil).
		blocked = resolveForValidation(filepath.Clean(blocked))
		if abs == blocked || strings.HasPrefix(abs, blocked+string(os.PathSeparator)) {
			return fmt.Errorf("download directory must not point at %s", blocked)
		}
	}
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		return fmt.Errorf("download directory is not a directory: %s", trimmed)
	}
	return nil
}

// pathWithinDir reports whether absPath equals dir or lives inside it.
// Both arguments must already be absolute and clean.
func pathWithinDir(absPath, dir string) bool {
	return absPath == dir || strings.HasPrefix(absPath, dir+string(os.PathSeparator))
}

// isAllowedOpenPath jails the open-file bridge to the user's download folder
// (including monthly subfolders) and the internal preview temp dir, so page
// JavaScript cannot ask the native side to open arbitrary files such as
// /etc/passwd with the default application.
func isAllowedOpenPath(filePath string) bool {
	trimmed := strings.TrimSpace(filePath)
	if trimmed == "" {
		return false
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return false
	}
	abs = resolveForValidation(abs)
	if dl := strings.TrimSpace(loadSettings().DownloadDir); dl != "" {
		if dlAbs, err := filepath.Abs(dl); err == nil {
			dlAbs = resolveForValidation(dlAbs)
			if pathWithinDir(abs, dlAbs) {
				return true
			}
		}
	}
	previewDir := filepath.Join(os.TempDir(), "WhatsAppDeskPreview")
	if prevAbs, err := filepath.Abs(previewDir); err == nil {
		// Resolve prevAbs too: os.TempDir() is /var/... on macOS, which
		// resolves to /private/var/..., so an unresolved prefix can never
		// match the already-resolved candidate and preview files would always
		// be refused.
		if pathWithinDir(abs, resolveForValidation(prevAbs)) {
			return true
		}
	}
	return false
}

// maxAttachmentBytes bounds a single saved attachment after base64
// decoding. The bridge design buffers the whole file in RAM (JS string +
// Go string + decoded bytes ≈ 2.7x), so an unbounded payload is an instant
// OOM; 1 GB is far above legitimate chat attachments while keeping worst-
// case memory use survivable on a desktop.
var maxAttachmentBytes int64 = 1 << 30

func saveDownloadedFileToDir(targetDir, filename, dataURI string) (string, error) {
	// Validate before MkdirAll so a hostile settings.json can never cause a
	// sensitive directory (e.g. ~/.config/autostart) to be created/populated.
	if err := validateDownloadDir(targetDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create target directory: %w", err)
	}

	// Sanitize filename against directory traversal
	filename = filepath.Base(filepath.Clean(filename))
	if filename == "." || filename == "/" || filename == "" {
		filename = "download"
	}

	// Reject oversized payloads BEFORE decoding: base64 expands ~4/3, so
	// checking the encoded length avoids the transient 2x memory spike of a
	// decode-then-check.
	payload := dataURI
	if idx := strings.Index(dataURI, ";base64,"); idx != -1 {
		payload = dataURI[idx+8:]
	}
	if int64(len(payload))*3/4 > maxAttachmentBytes {
		return "", fmt.Errorf("attachment rejected: size exceeds %d-byte limit", maxAttachmentBytes)
	}

	// Extract and decode base64 payload
	var rawBytes []byte
	var err error
	idx := strings.Index(dataURI, ";base64,")
	if idx != -1 {
		rawBytes, err = base64.StdEncoding.DecodeString(dataURI[idx+8:])
	} else {
		rawBytes, err = base64.StdEncoding.DecodeString(dataURI)
	}
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	return saveDownloadedBytesToDir(targetDir, filename, rawBytes)
}

func fileMatchesBytes(path string, expected []byte, expectedHash [sha256.Size]byte) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() != int64(len(expected)) {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}
	return bytes.Equal(h.Sum(nil), expectedHash[:])
}

func saveDownloadedBytesToDir(targetDir, filename string, rawBytes []byte) (string, error) {
	downloadFileMu.Lock()
	defer downloadFileMu.Unlock()

	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	if base == "" {
		base = "download"
	}
	expectedHash := sha256.Sum256(rawBytes)

	for counter := 0; ; counter++ {
		candidateName := filename
		if counter > 0 {
			candidateName = fmt.Sprintf("%s (%d)%s", base, counter, ext)
		}
		targetPath := filepath.Join(targetDir, candidateName)

		if fileMatchesBytes(targetPath, rawBytes, expectedHash) {
			return targetPath, nil
		}

		out, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("failed to create download: %w", err)
		}

		_, copyErr := io.Copy(out, bytes.NewReader(rawBytes))
		closeErr := out.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(targetPath)
			if copyErr != nil {
				return "", fmt.Errorf("failed to save file: %w", copyErr)
			}
			return "", fmt.Errorf("failed to close saved file: %w", closeErr)
		}
		return targetPath, nil
	}
}

func openFolderInFileManager(folderPath string) error {
	if _, err := os.Stat(folderPath); os.IsNotExist(err) {
		_ = os.MkdirAll(folderPath, 0755)
	}
	if runtime.GOOS == "darwin" {
		return exec.Command("open", folderPath).Start()
	} else if runtime.GOOS == "windows" {
		return exec.Command("explorer.exe", folderPath).Start()
	} else if runtime.GOOS == "linux" {
		return exec.Command("xdg-open", folderPath).Start()
	}
	return nil
}

func previewDocument(filename, dataURI string) (string, error) {
	tempDir := filepath.Join(os.TempDir(), "WhatsAppDeskPreview")
	_ = os.MkdirAll(tempDir, 0755)

	targetPath, err := saveDownloadedFileToDir(tempDir, filename, dataURI)
	if err != nil {
		return "", err
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", targetPath)
	case "windows":
		// rundll32 is a GUI-subsystem binary, so unlike "cmd /c start" it never
		// flashes a console window when opening the previewed file.
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetPath)
	default:
		cmd = exec.Command("xdg-open", targetPath)
	}
	if cmd != nil {
		_ = cmd.Start()
	}
	return targetPath, nil
}

func openFileInDefaultApp(filePath string) bool {
	if filePath == "" {
		return false
	}
	// Jail: the path arrives via the JS bridge, so only files inside the
	// download folder or the internal preview dir may be opened.
	if !isAllowedOpenPath(filePath) {
		return false
	}
	if _, err := os.Stat(filePath); err != nil {
		return false
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", filePath)
	case "windows":
		// rundll32 avoids the console flash of "cmd /c start" (GUI subsystem).
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", filePath)
	default:
		cmd = exec.Command("xdg-open", filePath)
	}
	if cmd != nil {
		_ = cmd.Start()
		return true
	}
	return false
}
