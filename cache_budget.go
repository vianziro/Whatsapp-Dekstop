package main

// Cross-platform disk-cache budget enforcement. Each platform file calls
// enforceDiskCacheCapFrom() at startup and on minimize/close with its own
// engine's cache directories; this shared helper does the measuring and, when
// the budget is exceeded, deletes only cache subdirectories — never profile
// data (cookies/localStorage/IndexedDB live elsewhere).

import (
	"os"
	"path/filepath"
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
func enforceDiskCacheCapFrom(roots, purgeTargets []string, phase string) {
	if !debugEnabled() {
		go func() {
			if dirSizeTotal(roots) <= diskCacheCapBytes {
				return
			}
			for _, target := range purgeTargets {
				_ = os.RemoveAll(target)
				_ = os.MkdirAll(target, 0755)
			}
		}()
		return
	}
	go func() {
		start := time.Now()
		total := dirSizeTotal(roots)
		if total <= diskCacheCapBytes {
			cacheDebugLog("cache %s: %.0f MB <= cap, no purge", phase, float64(total)/1024/1024)
			return
		}
		for _, target := range purgeTargets {
			_ = os.RemoveAll(target)
			_ = os.MkdirAll(target, 0755)
		}
		cacheDebugLog("cache %s: purged %.0f MB (>512 MB cap) in %s",
			phase, float64(total)/1024/1024, time.Since(start).Round(time.Millisecond))
	}()
}

func dirSizeTotal(paths []string) int64 {
	var total int64
	for _, p := range paths {
		total += dirSizeBytes(p)
	}
	return total
}
