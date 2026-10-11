//go:build !darwin && !linux && !windows

package main

// readProcessRSSKB has no implementation on this platform. Reporting false makes
// the diagnostics log say "unavailable" rather than print a number that is not
// the process's memory.
func readProcessRSSKB() (int64, string, bool) { return 0, "unavailable", false }
