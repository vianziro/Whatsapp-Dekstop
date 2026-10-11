//go:build darwin

package main

/*
#include <mach/mach.h>
#include <mach/task_info.h>

// whatsappDeskResidentKB asks the kernel for this task's resident size. Reading
// it in-process avoids forking a helper and avoids depending on PATH, which a
// double-clicked bundle does not fully inherit. Returns -1 on failure.
static long long whatsappDeskResidentKB(void) {
	struct mach_task_basic_info info;
	mach_msg_type_number_t count = MACH_TASK_BASIC_INFO_COUNT;
	kern_return_t kr = task_info(mach_task_self(), MACH_TASK_BASIC_INFO,
	                             (task_info_t)&info, &count);
	if (kr != KERN_SUCCESS) {
		return -1;
	}
	return (long long)(info.resident_size / 1024);
}
*/
import "C"

// readProcessRSSKB returns this process's resident set size in kilobytes, read
// from the kernel rather than from a helper process.
func readProcessRSSKB() (int64, string, bool) {
	kb := int64(C.whatsappDeskResidentKB())
	if kb <= 0 {
		return 0, "task_info", false
	}
	return kb, "task_info", true
}
