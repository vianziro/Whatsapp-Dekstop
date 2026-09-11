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

	// Prepare background swap and restart command
	script := fmt.Sprintf(`(sleep 1 && rm -rf %q && cp -R %q %q && rm -rf %q %q && open %q) &`,
		appBundle, newAppPath, appBundle, extractDir, zipPath, appBundle)

	cmd := exec.Command("sh", "-c", script)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start restart script: %w", err)
	}

	os.Exit(0)
	return nil
}

func applyUpdate(downloadedFile string) error {
	return applyUpdateDarwin(downloadedFile)
}
