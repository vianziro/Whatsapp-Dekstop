//go:build linux

package main

import (
	"fmt"
	"os"
)

// readProcessRSSKB reads the resident page count from /proc/self/statm: one read
// with no child process involved.
func readProcessRSSKB() (int64, string, bool) {
	out, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, "statm", false
	}
	var total, resident int64
	if _, err := fmt.Sscanf(string(out), "%d %d", &total, &resident); err != nil || resident <= 0 {
		return 0, "statm", false
	}
	return resident * int64(os.Getpagesize()) / 1024, "statm", true
}
