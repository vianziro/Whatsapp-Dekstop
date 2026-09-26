//go:build linux

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestSafeExtractMode(t *testing.T) {
	cases := []struct {
		mode  int64
		isDir bool
		want  os.FileMode
	}{
		{04755, false, 0744}, // setuid binary stays owner-runnable, loses setuid
		{02775, true, 0755},  // setgid dir loses setgid
		{0644, false, 0644},
		{0755, false, 0744}, // group/other exec not needed to run our binary
		{0600, false, 0600},
		{01777, true, 0755}, // sticky dir loses sticky
	}
	for _, c := range cases {
		if got := safeExtractMode(c.mode, c.isDir); got != c.want {
			t.Errorf("safeExtractMode(%#o, %v) = %#o, want %#o", c.mode, c.isDir, got, c.want)
		}
	}
}

func TestExtractTarGzMasksElevatedModes(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	entries := []struct {
		name string
		mode int64
		body string
		dir  bool
	}{
		{"evil-suid", 04755, "binary", false},
		{"plain.txt", 0644, "text", false},
		{"run.sh", 0755, "script", false},
		{"sticky-dir", 02775, "", true},
	}
	for _, e := range entries {
		flag := byte(tar.TypeReg)
		if e.dir {
			flag = byte(tar.TypeDir)
		}
		if err := tw.WriteHeader(&tar.Header{Name: e.name, Mode: e.mode, Size: int64(len(e.body)), Typeflag: flag}); err != nil {
			t.Fatal(err)
		}
		if !e.dir {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(t.TempDir(), "payload.tar.gz")
	if err := os.WriteFile(archive, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if err := extractTarGz(archive, dest); err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		info, err := os.Stat(filepath.Join(dest, e.name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&os.ModeSetuid != 0 || info.Mode()&os.ModeSetgid != 0 || info.Mode()&os.ModeSticky != 0 {
			t.Errorf("%s extracted with elevated bits: %s", e.name, info.Mode())
		}
	}
	for name, want := range map[string]os.FileMode{"evil-suid": 0744, "plain.txt": 0644, "run.sh": 0744, "sticky-dir": 0755} {
		info, err := os.Stat(filepath.Join(dest, name))
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %#o, want %#o", name, got, want)
		}
	}
}
