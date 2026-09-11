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
