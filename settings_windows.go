//go:build windows

package main

import (
	"strings"
)

func chooseFolderDialog() (string, error) {
	script := `Add-Type -AssemblyName System.Windows.Forms; $f = New-Object System.Windows.Forms.FolderBrowserDialog; $f.Description = 'Pilih Folder Penyimpanan File WhatsApp'; if ($f.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { Write-Output $f.SelectedPath }`
	// hiddenCommand keeps the picker dialog itself (a GUI window) but hides
	// the powershell console that would otherwise flash behind it.
	out, err := hiddenCommand("powershell", "-NoProfile", "-Command", script).Output()
	if err != nil {
		return "", err
	}
	res := strings.TrimSpace(string(out))
	return res, nil
}
