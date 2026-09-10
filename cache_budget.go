package main

// Cross-platform disk-cache budget enforcement. Each platform file calls
// enforceDiskCacheCapFrom() at startup and on minimize/close with its own
// engine's cache directories; this shared helper does the measuring and, when
// the budget is exceeded, deletes only cache subdirectories — never profile
// data (cookies/localStorage/IndexedDB live elsewhere).

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const diskCacheCapBytes = int64(512) * 1024 * 1024

func dirSizeBytes(path string) int64 {
	var total int64
	filepath.WalkDir(path, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if info, err := entry.Info(); err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

// cacheSubdirs removes only the given subdirectories' contents when the total
// size of all watched roots exceeds the cap. Runs asynchronously.
// ebWebViewCacheBusy reports whether the WebView2 browser process holds any
// file inside the cache home open. Deleting Chromium cache directories while
// the engine is running can corrupt the profile (locked entries are skipped
// mid-recursive-delete), so on Windows the purge is skipped whenever the
// runtime is alive; the next app start clears it safely instead.
func ebWebViewCacheBusy() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	out, err := exec.Command("powershell", "-NoProfile", "-Command",
		`Get-Process msedgewebview2 -ErrorAction SilentlyContinue | Measure-Object | Select-Object -ExpandProperty Count`).Output()
	if err != nil {
		// Cannot tell: assume busy rather than risk a corrupting delete.
		return true
	}
	n, parseErr := strconv.Atoi(strings.TrimSpace(string(out)))
	if parseErr != nil {
		return true
	}
	return n > 0
}

// enforceDiskCacheCapSync runs the same budget check on the calling goroutine.
// Used at startup, where the WebView2 engine has not been created yet: the
// delete must complete before the engine starts opening those files.
func enforceDiskCacheCapSync(roots, purgeTargets []string, phase string) {
	total := dirSizeTotal(roots)
	if total <= diskCacheCapBytes {
		cacheDebugLog("cache %s: %.0f MB <= cap, no purge", phase, float64(total)/1024/1024)
		return
	}
	if ebWebViewCacheBusy() {
		cacheDebugLog("cache %s: engine already running; skipping sync purge", phase)
		return
	}
	start := time.Now()
	for _, target := range purgeTargets {
		_ = os.RemoveAll(target)
		_ = os.MkdirAll(target, 0755)
	}
	cacheDebugLog("cache %s: purged %.0f MB (>512 MB cap) in %s",
		phase, float64(total)/1024/1024, time.Since(start).Round(time.Millisecond))
}

func enforceDiskCacheCapFrom(roots, purgeTargets []string, phase string) {
	purge := func() bool {
		total := dirSizeTotal(roots)
		if total <= diskCacheCapBytes {
			cacheDebugLog("cache %s: %.0f MB <= cap, no purge", phase, float64(total)/1024/1024)
			return false
		}
		if ebWebViewCacheBusy() {
			// Engine running: deleting the live cache dir risks profile
			// corruption. Defer to the next startup, which purges pre-launch.
			cacheDebugLog("cache %s: %.0f MB over cap but engine running; deferring purge", phase, float64(total)/1024/1024)
			return false
		}
		start := time.Now()
		for _, target := range purgeTargets {
			_ = os.RemoveAll(target)
			_ = os.MkdirAll(target, 0755)
		}
		cacheDebugLog("cache %s: purged %.0f MB (>512 MB cap) in %s",
			phase, float64(total)/1024/1024, time.Since(start).Round(time.Millisecond))
		return true
	}
	go func() {
		if !debugEnabled() {
			// Silent path: still respect the engine-busy guard above; only the
			// logging differs between debug and non-debug builds.
			total := dirSizeTotal(roots)
			if total <= diskCacheCapBytes {
				return
			}
			if ebWebViewCacheBusy() {
				return
			}
			for _, target := range purgeTargets {
				_ = os.RemoveAll(target)
				_ = os.MkdirAll(target, 0755)
			}
			return
		}
		purge()
	}()
}

func dirSizeTotal(paths []string) int64 {
	var total int64
	for _, p := range paths {
		total += dirSizeBytes(p)
	}
	return total
}
