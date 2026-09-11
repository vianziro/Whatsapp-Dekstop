//go:build darwin

package main

/*
#cgo darwin CFLAGS: -x objective-c
#cgo darwin LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#include <stdlib.h>

static char* chooseDownloadFolder(void) {
    __block NSString *selectedPath = nil;
    void (^showPanel)(void) = ^{
        NSOpenPanel *panel = [NSOpenPanel openPanel];
        panel.title = @"Pilih Folder Unduhan";
        panel.message = @"Pilih lokasi penyimpanan berkas dari WhatsApp Desk.";
        panel.prompt = @"Pilih Folder";
        panel.canChooseFiles = NO;
        panel.canChooseDirectories = YES;
        panel.allowsMultipleSelection = NO;
        panel.canCreateDirectories = YES;
        NSString *downloads = [NSSearchPathForDirectoriesInDomains(NSDownloadsDirectory, NSUserDomainMask, YES) firstObject];
        if (downloads) panel.directoryURL = [NSURL fileURLWithPath:downloads];
        [NSApp activateIgnoringOtherApps:YES];
        if ([panel runModal] == NSModalResponseOK) selectedPath = panel.URL.path;
    };

    if ([NSThread isMainThread]) showPanel();
    else dispatch_sync(dispatch_get_main_queue(), showPanel);
    if (!selectedPath) return NULL;
    return strdup(selectedPath.UTF8String);
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

func chooseFolderDialog() (string, error) {
	path := C.chooseDownloadFolder()
	if path == nil {
		return "", errors.New("pemilihan folder dibatalkan")
	}
	defer C.free(unsafe.Pointer(path))
	return C.GoString(path), nil
}
