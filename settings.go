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
	DownloadDir       string `json:"download_dir"`
	NotifyOnDownload  bool   `json:"notify_on_download"`
	Theme             string `json:"theme"` // "dark", "light", "system"
	OrganizeByMonth   bool   `json:"organize_by_month"`
	SpellCheckEnabled bool   `json:"spell_check_enabled"`
	SpellCheckLang    string `json:"spell_check_lang"`
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
		DownloadDir:       getDefaultDownloadDir(),
		NotifyOnDownload:  true,
		Theme:             "dark",
		SpellCheckEnabled: true,
		SpellCheckLang:    "auto",
	}
	data, err := os.ReadFile(getSettingsFilePath())
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, s)
	if strings.TrimSpace(s.DownloadDir) == "" {
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

// fileExistsInDownloadDir reports whether filename exists anywhere the saver
// could have placed it: directly in the downloads folder, or inside any
// YYYY-MM subfolder created by monthly organization. The badge layer uses
// this, so it must not depend on the current preference value.
func fileExistsInDownloadDir(filename string) bool {
	name := filepath.Base(strings.TrimSpace(filename))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return false
	}
	s := loadSettings()
	dir := s.DownloadDir
	if strings.TrimSpace(dir) == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
		return true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() || len(e.Name()) != 7 || e.Name()[4] != '-' {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, e.Name(), name)); err == nil {
			return true
		}
	}
	return false
}

func saveDownloadedFileToDir(targetDir, filename, dataURI string) (string, error) {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create target directory: %w", err)
	}

	// Sanitize filename against directory traversal
	filename = filepath.Base(filepath.Clean(filename))
	if filename == "." || filename == "/" || filename == "" {
		filename = "download"
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
