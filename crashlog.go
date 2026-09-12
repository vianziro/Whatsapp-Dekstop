package main

// Local crash reporting. A panic anywhere in the app is caught, summarized
// with the stack trace, and appended to wa_crash.log in the profile directory
// — no network, no telemetry. Users can attach one file to a bug report
// instead of screenshots from Console.app.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

func crashLogPath() string {
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		return filepath.Join(home, "Library", "Application Support", "WhatsAppDesk", "wa_crash.log")
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "WhatsAppDesk", "wa_crash.log")
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		return filepath.Join(home, ".config", "whatsapp-desk", "wa_crash.log")
	}
}

func writeCrashReport(context string, recovered interface{}) {
	stack := debug.Stack()
	defer func() { _ = recover() }() // never let logging itself crash the app
	_ = os.MkdirAll(filepath.Dir(crashLogPath()), 0755)
	f, err := os.OpenFile(crashLogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "\n===== crash %s | app v%s | %s =====\nrecovered in %s: %v\n%s\n",
		time.Now().Format(time.RFC3339), appVersion, runtime.GOOS, context, recovered, string(stack))
}

// guardGoroutine wraps a background goroutine body so a panic there is logged
// locally instead of killing the whole process.
func guardGoroutine(name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			writeCrashReport("goroutine:"+name, r)
		}
	}()
	fn()
}

// readCrashTail returns the last maxChars of the local crash log with the
// home directory scrubbed, so no usernames or profile paths can leak into a
// pasted issue report.
func readCrashTail(maxChars int) string {
	data, err := os.ReadFile(crashLogPath())
	if err != nil || len(data) == 0 {
		return ""
	}
	if home, herr := os.UserHomeDir(); herr == nil && home != "" {
		data = []byte(strings.ReplaceAll(string(data), home, "~"))
	}
	s := string(data)
	if maxChars > 0 && len(s) > maxChars {
		s = s[len(s)-maxChars:]
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
	}
	return s
}

// pendingCrashReport returns crash-log content the user hasn't been nudged
// about yet, or "" when there is nothing new. Marking happens explicitly via
// markCrashNotified so the startup nudge fires exactly once per crash.
func pendingCrashReport() string {
	info, err := os.Stat(crashLogPath())
	if err != nil || info.Size() == 0 {
		return ""
	}
	if info.ModTime().Unix() <= loadSettings().LastCrashNotified {
		return ""
	}
	return readCrashTail(1500)
}

func markCrashNotified() bool {
	s := loadSettings()
	if info, err := os.Stat(crashLogPath()); err == nil {
		s.LastCrashNotified = info.ModTime().Unix()
		_ = saveSettings(s)
		return true
	}
	return false
}
