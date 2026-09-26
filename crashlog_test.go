package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCrashLogRotatesWhenFull(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	prev := maxCrashLogBytes
	maxCrashLogBytes = 1024
	t.Cleanup(func() { maxCrashLogBytes = prev })

	path := crashLogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	filler := make([]byte, 2048)
	if err := os.WriteFile(path, filler, 0644); err != nil {
		t.Fatal(err)
	}

	writeCrashReport("test-rotation", "boom")

	backup, err := os.Stat(path + ".1")
	if err != nil {
		t.Fatalf("expected rotated backup: %v", err)
	}
	if backup.Size() != int64(len(filler)) {
		t.Fatalf("backup size = %d, want %d", backup.Size(), len(filler))
	}
	current, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if current.Size() >= int64(len(filler)) {
		t.Fatalf("current log not rotated, size = %d", current.Size())
	}
}

func TestCrashLogKeepsSingleSmallFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	prev := maxCrashLogBytes
	maxCrashLogBytes = 1024
	t.Cleanup(func() { maxCrashLogBytes = prev })

	writeCrashReport("test-small", "boom")

	if _, err := os.Stat(crashLogPath() + ".1"); !os.IsNotExist(err) {
		t.Fatal("small log must not be rotated")
	}
	if _, err := os.Stat(crashLogPath()); err != nil {
		t.Fatalf("current log missing: %v", err)
	}
}
