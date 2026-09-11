//go:build linux

package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func extractTarGz(srcFile, destDir string) error {
	f, err := os.Open(srcFile)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	_ = os.MkdirAll(destDir, 0755)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		targetPath := filepath.Join(destDir, header.Name)
		cleanDest := filepath.Clean(destDir)
		cleanTarget := filepath.Clean(targetPath)
		if !strings.HasPrefix(cleanTarget, cleanDest+string(os.PathSeparator)) && cleanTarget != cleanDest {
			return fmt.Errorf("tar entry escapes destination: %s", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			_ = os.MkdirAll(targetPath, 0755)
		case tar.TypeReg:
			_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}
	return nil
}

func copyExecutableAtomically(srcPath, destPath string) error {
	destDir := filepath.Dir(destPath)
	tempFile, err := os.CreateTemp(destDir, ".whatsapp-desk-update-*")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	cleanup := true
	defer func() {
		_ = tempFile.Close()
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()

	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	if _, err := io.Copy(tempFile, src); err != nil {
		return err
	}
	if err := tempFile.Sync(); err != nil {
		return err
	}
	if err := tempFile.Chmod(0755); err != nil {
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, destPath); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func replaceLinuxExecutable(srcPath, destPath string) error {
	if err := copyExecutableAtomically(srcPath, destPath); err == nil {
		return nil
	}

	pkexecPath, err := exec.LookPath("pkexec")
	if err != nil {
		return fmt.Errorf("application location is not writable and pkexec is unavailable; reinstall the latest DEB package")
	}
	// Positional parameters keep paths out of the shell program text.
	cmd := exec.Command(pkexecPath, "/bin/sh", "-c",
		`set -eu; tmp="$2.update.$$"; trap 'rm -f "$tmp"' EXIT; install -m 0755 "$1" "$tmp"; mv -f "$tmp" "$2"`,
		"whatsapp-desk-updater", srcPath, destPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("authorized update failed: %s", message)
	}
	return nil
}

func applyUpdateLinux(downloadedFile string) error {
	execPath, err := os.Executable()
	if err != nil {
		return err
	}

	newBinaryPath := downloadedFile
	if strings.HasSuffix(strings.ToLower(downloadedFile), ".tar.gz") || strings.HasSuffix(strings.ToLower(downloadedFile), ".tgz") {
		extractDir := filepath.Join(os.TempDir(), "whatsapp_linux_extracted")
		_ = os.RemoveAll(extractDir)
		if err := extractTarGz(downloadedFile, extractDir); err != nil {
			return fmt.Errorf("tar extract error: %w", err)
		}

		// Find binary in extracted directory
		entries, _ := os.ReadDir(extractDir)
		for _, e := range entries {
			if !e.IsDir() && (strings.Contains(strings.ToLower(e.Name()), "whatsapp") || e.Name() == "whatsapp-desk" || e.Name() == "whatsapp-desktop-light") {
				newBinaryPath = filepath.Join(extractDir, e.Name())
				break
			}
		}
	}

	if _, err := os.Stat(newBinaryPath); err != nil {
		return fmt.Errorf("new binary not found: %w", err)
	}

	if err := replaceLinuxExecutable(newBinaryPath, execPath); err != nil {
		return fmt.Errorf("failed to replace application binary: %w", err)
	}
	_ = os.Remove(downloadedFile)

	cmd := exec.Command(execPath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("updated but failed to restart: %w", err)
	}
	os.Exit(0)
	return nil
}

func applyUpdate(downloadedFile string) error {
	return applyUpdateLinux(downloadedFile)
}
