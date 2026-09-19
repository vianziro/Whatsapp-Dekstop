//go:build darwin

package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func unzipArchive(srcZip, destDir string) error {
	r, err := zip.OpenReader(srcZip)
	if err != nil {
		return err
	}
	defer r.Close()

	_ = os.MkdirAll(destDir, 0755)

	cleanDest := filepath.Clean(destDir)
	for _, f := range r.File {
		targetPath := filepath.Join(destDir, f.Name)
		cleanTarget := filepath.Clean(targetPath)
		if !strings.HasPrefix(cleanTarget, cleanDest+string(os.PathSeparator)) && cleanTarget != cleanDest {
			continue
		}

		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(targetPath, f.Mode())
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		if f.Mode()&os.ModeSymlink != 0 {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			linkTarget, _ := io.ReadAll(rc)
			rc.Close()
			_ = os.Symlink(string(linkTarget), targetPath)
			continue
		}

		outFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}

		// Ensure executables inside macOS bundle have executable permissions
		if strings.Contains(targetPath, "Contents/MacOS/") {
			_ = os.Chmod(targetPath, 0755)
		}
	}
	return nil
}

func applyUpdateDarwin(zipPath string) error {
	appBundle := getAppBundlePath()
	if appBundle == "" {
		return fmt.Errorf("unable to determine app bundle path")
	}

	extractDir := filepath.Join(os.TempDir(), "whatsapp_update_extracted")
	_ = os.RemoveAll(extractDir)
	if err := unzipArchive(zipPath, extractDir); err != nil {
		return fmt.Errorf("unzip failed: %w", err)
	}

	// Look for WhatsApp Desk.app inside extracted dir
	newAppPath := filepath.Join(extractDir, "WhatsApp Desk.app")
	if _, err := os.Stat(newAppPath); err != nil {
		newAppPath = filepath.Join(extractDir, "WhatsApp Desktop Light.app")
	}
	if _, err := os.Stat(newAppPath); err != nil {
		newAppPath = filepath.Join(extractDir, "WhatsApp.app")
	}
	if _, err := os.Stat(newAppPath); err != nil {
		entries, _ := os.ReadDir(extractDir)
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".app") {
				newAppPath = filepath.Join(extractDir, e.Name())
				break
			}
		}
	}

	if _, err := os.Stat(newAppPath); err != nil {
		return fmt.Errorf("extracted app not found at %s", newAppPath)
	}

	// Use a script with positional arguments instead of interpolating paths into
	// shell source. App paths can contain spaces, quotes, $, or other shell
	// metacharacters, and updates must not turn those paths into commands.
	scriptPath := filepath.Join(os.TempDir(), fmt.Sprintf("whatsapp_update_%d.sh", os.Getpid()))
	logPath := filepath.Join(os.TempDir(), "WhatsAppDesk-update.log")
	script := `#!/bin/sh
set -eu
old_app="$1"
new_app="$2"
extract_dir="$3"
zip_path="$4"
log_path="$5"
script_path="$6"
backup_path="${old_app}.old.$$"
trap 'rm -f "$script_path"' EXIT
{
  sleep 1
  mv "$old_app" "$backup_path"
  if ! /usr/bin/ditto "$new_app" "$old_app"; then
    rm -rf "$old_app"
    mv "$backup_path" "$old_app"
    echo "WhatsApp Desk update failed while copying the new app." > "$log_path"
    exit 1
  fi
  rm -rf "$backup_path" "$extract_dir" "$zip_path"
  /usr/bin/open "$old_app"
} >> "$log_path" 2>&1
`
	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		return fmt.Errorf("failed to prepare update helper: %w", err)
	}

	cmd := exec.Command("/bin/sh", scriptPath, appBundle, newAppPath, extractDir, zipPath, logPath, scriptPath)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(scriptPath)
		return fmt.Errorf("failed to start restart script: %w", err)
	}

	os.Exit(0)
	return nil
}

func applyUpdate(downloadedFile string) error {
	return applyUpdateDarwin(downloadedFile)
}
