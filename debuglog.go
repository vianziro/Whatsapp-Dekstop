package main

// Optional internal diagnostics. Enabled with WA_DESK_DEBUG=1; everything is a
// no-op otherwise. Logs stay on this machine (profile dir / wa_debug.log) and
// are never shipped anywhere.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

var (
	debugLogMu     sync.Mutex
	debugLogFile   *os.File
	debugStartedAt = time.Now()
)

func debugEnabled() bool {
	return os.Getenv("WA_DESK_DEBUG") == "1"
}

func debugLogPath() string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDirOrDot(), "Library", "Application Support", "WhatsAppDesk", "wa_debug.log")
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "WhatsAppDesk", "wa_debug.log")
	default:
		return filepath.Join(homeDirOrDot(), ".config", "whatsapp-desk", "wa_debug.log")
	}
}

func homeDirOrDot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

func cacheDebugLog(format string, args ...interface{}) {
	if !debugEnabled() {
		return
	}
	debugLogMu.Lock()
	defer debugLogMu.Unlock()
	if debugLogFile == nil {
		_ = os.MkdirAll(filepath.Dir(debugLogPath()), 0755)
		f, err := os.OpenFile(debugLogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		debugLogFile = f
	}
	fmt.Fprintf(debugLogFile, "[%s +%4.1fs] %s\n",
		time.Now().Format("15:04:05"), time.Since(debugStartedAt).Seconds(),
		fmt.Sprintf(format, args...))
}

// debugLogProcessStats writes the current process RSS plus, on darwin, the
// resident size of the WebKit child processes. Light: one `ps` invocation.
func debugLogProcessStats(phase string) {
	if !debugEnabled() {
		return
	}
	out, err := os.ReadFile("/proc/self/statm")
	rss := "n/a"
	if err == nil && len(out) > 0 {
		var total, resident uint64
		if _, err := fmt.Sscanf(string(out), "%d %d", &total, &resident); err == nil {
			rss = fmt.Sprintf("%dMB", resident*uint64(pageSize())/1024/1024)
		}
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		rss = fmt.Sprintf("%dMB", approxSelfRSS())
	}
	cacheDebugLog("stats phase=%s rss=%s goroutines=%d", phase, rss, runtime.NumGoroutine())
}

func pageSize() int {
	return 4096
}

func approxSelfRSS() int64 {
	// Cheap, portable-enough approximation for diagnostics only.
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return int64(m.Sys / 1024 / 1024)
}
