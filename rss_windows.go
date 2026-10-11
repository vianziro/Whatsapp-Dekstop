//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var procGetProcessMemoryInfo = syscall.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

// processMemoryCounters mirrors PROCESS_MEMORY_COUNTERS. The layout is spelled
// out rather than assumed: cb and PageFaultCount are DWORDs while the rest are
// SIZE_T, so the struct is 72 bytes on 64-bit and 40 on 32-bit.
type processMemoryCounters struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

// readProcessRSSKB returns this process's working set — the Windows equivalent
// of resident set size — in kilobytes. It reports false rather than a guess when
// the call fails, so the log says unavailable instead of a number that is not
// memory.
func readProcessRSSKB() (int64, string, bool) {
	var counters processMemoryCounters
	counters.Cb = uint32(unsafe.Sizeof(counters))

	// procGetCurrentProcess is declared once, in app_windows.go.
	h, _, _ := procGetCurrentProcess.Call()
	if h == 0 {
		return 0, "psapi", false
	}
	ok, _, _ := procGetProcessMemoryInfo.Call(
		h, uintptr(unsafe.Pointer(&counters)), uintptr(counters.Cb))
	if ok == 0 {
		return 0, "psapi", false
	}
	return int64(counters.WorkingSetSize / 1024), "psapi", true
}
