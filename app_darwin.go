//go:build darwin

package main

/*
#cgo darwin CFLAGS: -x objective-c
#cgo darwin LDFLAGS: -framework Cocoa -framework WebKit -framework PDFKit

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <PDFKit/PDFKit.h>
#include <stdlib.h>

// Declared early so the memory purge routine below can reach the live WKWebView
// instance (and its real, already-attached website data store) instead of only
// posting a notification that WebKit does not actually observe.
static WKWebView* g_mainWebView = nil;

static void configureWebKitMemoryLimits(void) {
    static dispatch_once_t onceToken;
    dispatch_once(&onceToken, ^{
        // Allocate healthy persistent disk & RAM cache so WhatsApp Status/Story videos and chat media buffer smoothly
        NSArray *paths = NSSearchPathForDirectoriesInDomains(NSCachesDirectory, NSUserDomainMask, YES);
        NSString *cachePath = [paths.firstObject stringByAppendingPathComponent:@"com.whatsapp.desk/WebCache"];
        NSURLCache *sharedCache = [[NSURLCache alloc] initWithMemoryCapacity:24 * 1024 * 1024
                                                                diskCapacity:256 * 1024 * 1024
                                                                    diskPath:cachePath];
        [NSURLCache setSharedURLCache:sharedCache];
    });
}

// Disk cache budget. WebKit's NetworkCache (the one WKWebView actually uses)
// grows unbounded on disk; measure it and wipe it when it exceeds the cap.
// 512 MB total across both cache homes keeps media previews snappy while
// guaranteeing the profile can never balloon unnoticed for months.
static const long long kWhatsAppDeskCacheCapBytes = 512LL * 1024 * 1024;

// enumeratorAtURL (not enumeratorAtPath): the path-based enumerator's
// nextObject returns NSString paths, and a previous version treated those as
// NSDictionary and called -fileSize on them — an unrecognized selector that
// raised an Objective-C exception on the utility queue and aborted the whole
// app (SIGABRT ~8s after launch whenever a background purge ran). The URL
// enumerator hands out real attribute values instead.
static long long directorySizeBytes(NSString* path) {
    @try {
        NSURL* dirURL = [NSURL fileURLWithPath:path];
        NSDirectoryEnumerator* enumerator = [[NSFileManager defaultManager]
            enumeratorAtURL:dirURL
            includingPropertiesForKeys:@[NSURLFileSizeKey, NSURLIsRegularFileKey]
            options:0
            errorHandler:nil];
        if (!enumerator) return 0;
        long long total = 0;
        for (NSURL* url in enumerator) {
            NSNumber* size = nil;
            if (![url getResourceValue:&size forKey:NSURLFileSizeKey error:nil]) continue;
            if (size) total += [size longLongValue];
        }
        return total;
    } @catch (NSException* e) {
        return 0;
    }
}

static long long whatsappDeskDiskCacheBytes(void) {
    @autoreleasepool {
        long long total = 0;
        NSArray* homes = @[
            [NSHomeDirectory() stringByAppendingPathComponent:@"Library/Caches/com.whatsapp.desk"],
            [NSHomeDirectory() stringByAppendingPathComponent:@"Library/WebKit/com.whatsapp.desk"],
        ];
        for (NSString* home in homes) total += directorySizeBytes(home);
        return total;
    }
}

// Runs on a background queue: called from purge paths (minimize/close), so a
// slow directory scan must never block the main thread. IMPORTANT: this block
// must not touch g_mainWebView or any live Objective-C object — if the app is
// quitting, the WebView is deallocated on the main thread while this block may
// still run, and messaging the dangling pointer raises an Objective-C
// exception (abort). Disk cache purge therefore happens via removeItemAtPath
// on plain paths only; the WebKit-internal purge stays on the calling thread.
static void enforceDiskCacheCap(void) {
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_UTILITY, 0), ^{
        @autoreleasepool {
            long long bytes = whatsappDeskDiskCacheBytes();
            if (bytes <= kWhatsAppDeskCacheCapBytes) return;
            // Over budget: drop our on-disk HTTP cache homes. These are plain
            // directories WebKit recreates as needed; session data (cookies/
            // localStorage/IndexedDB) lives elsewhere and is never touched.
            [[NSFileManager defaultManager] removeItemAtPath:
                [NSHomeDirectory() stringByAppendingPathComponent:@"Library/Caches/com.whatsapp.desk"]
                                                          error:nil];
            [[NSFileManager defaultManager] removeItemAtPath:
                [NSHomeDirectory() stringByAppendingPathComponent:@"Library/WebKit/com.whatsapp.desk/NetworkCache"]
                                                          error:nil];
        }
    });
}

static long long whatsappDeskDiskCacheBytesSync(void) {
    return whatsappDeskDiskCacheBytes();
}

static void purgeWebKitMemory(void) {
    @autoreleasepool {
        // Reclaim RAM by purging WebKit's in-memory resource cache only.
        // The on-disk HTTP cache is deliberately kept: it lives on disk, so wiping
        // it frees no meaningful RAM, but it forces WhatsApp Web to re-download its
        // bundles on the next use -- extra CPU and network traffic, plus a slow
        // first paint and chat-list scroll. Disk growth is bounded separately by
        // enforceDiskCacheCap() below. This intentionally excludes
        // cookies/localStorage/IndexedDB, so the logged-in session is never affected.
        if (g_mainWebView) {
            NSSet* memoryCacheTypes = [NSSet setWithObject:WKWebsiteDataTypeMemoryCache];
            WKWebsiteDataStore* store = g_mainWebView.configuration.websiteDataStore;
            [store removeDataOfTypes:memoryCacheTypes modifiedSince:[NSDate distantPast] completionHandler:^{}];
        }
        // Keep the on-disk cache under budget without discarding what is still useful.
        enforceDiskCacheCap();
    }
}

static void triggerNativeMemoryPurge(void) {
    purgeWebKitMemory();
}

@interface WhatsAppWindowDelegate : NSObject <NSWindowDelegate>
@end

@implementation WhatsAppWindowDelegate
- (BOOL)windowShouldClose:(NSWindow *)sender {
    [sender orderOut:nil];
    purgeWebKitMemory();
    return NO;
}
- (void)windowDidMiniaturize:(NSNotification *)notification {
    purgeWebKitMemory();
}
- (void)windowDidResignKey:(NSNotification *)notification {
    // Keep media playback and streaming buffers intact when switching windows
}
@end

@interface WhatsAppAppDelegate : NSObject <NSApplicationDelegate, NSUserNotificationCenterDelegate>
@property (assign) NSWindow *window;
@end

@implementation WhatsAppAppDelegate
- (BOOL)applicationShouldHandleReopen:(NSApplication *)sender hasVisibleWindows:(BOOL)flag {
    if (self.window) {
        [self.window makeKeyAndOrderFront:nil];
        [NSApp activateIgnoringOtherApps:YES];
    }
    return YES;
}

- (BOOL)userNotificationCenter:(NSUserNotificationCenter *)center shouldPresentNotification:(NSUserNotification *)notification {
    return YES;
}

- (void)userNotificationCenter:(NSUserNotificationCenter *)center didActivateNotification:(NSUserNotification *)notification {
    if (self.window) {
        [self.window makeKeyAndOrderFront:nil];
        [NSApp activateIgnoringOtherApps:YES];
    }
}
@end

static void postNativeMacNotification(const char* titleStr, const char* bodyStr) {
    @autoreleasepool {
        NSUserNotification *notification = [[NSUserNotification alloc] init];
        if (titleStr && strlen(titleStr) > 0) {
            notification.title = [NSString stringWithUTF8String:titleStr];
        } else {
            notification.title = @"WhatsApp Desk";
        }
        if (bodyStr && strlen(bodyStr) > 0) {
            notification.informativeText = [NSString stringWithUTF8String:bodyStr];
        }
        notification.soundName = NSUserNotificationDefaultSoundName;
        [[NSUserNotificationCenter defaultUserNotificationCenter] deliverNotification:notification];
    }
}

@interface WhatsAppUIDelegate : NSObject <WKUIDelegate>
@end

@implementation WhatsAppUIDelegate
// Without this, WKWebView silently cancels every <input type="file"> — the
// WhatsApp Web attach menu (Document, Photos & videos) closes with no dialog.
- (void)webView:(WKWebView *)webView runOpenPanelWithParameters:(WKOpenPanelParameters *)parameters initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(NSArray<NSURL *> *URLs))completionHandler {
    NSOpenPanel* panel = [NSOpenPanel openPanel];
    panel.canChooseFiles = YES;
    panel.canChooseDirectories = NO;
    panel.allowsMultipleSelection = parameters.allowsMultipleSelection;
    if ([panel runModal] == NSModalResponseOK) {
        completionHandler(panel.URLs);
    } else {
        completionHandler(nil);
    }
}
- (void)webView:(WKWebView *)webView requestMediaCapturePermissionForOrigin:(WKSecurityOrigin *)origin initiatedByFrame:(WKFrameInfo *)frame type:(WKMediaCaptureType)type decisionHandler:(void (^)(WKPermissionDecision decision))decisionHandler {
    decisionHandler(WKPermissionDecisionGrant);
}
- (WKWebView *)webView:(WKWebView *)webView createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration forNavigationAction:(WKNavigationAction *)navigationAction windowFeatures:(WKWindowFeatures *)windowFeatures {
    if (!navigationAction.targetFrame.isMainFrame) {
        NSURL *url = navigationAction.request.URL;
        if (url) {
            NSString *scheme = [[url scheme] lowercaseString];
            if (![scheme isEqualToString:@"blob"] && ![scheme isEqualToString:@"data"] && ![[url host] containsString:@"whatsapp."]) {
                [[NSWorkspace sharedWorkspace] openURL:url];
            }
        }
    }
    return nil;
}
@end

static WhatsAppAppDelegate* g_appDelegate = nil;
static WhatsAppWindowDelegate* g_windowDelegate = nil;
static WhatsAppUIDelegate* g_uiDelegate = nil;
static NSWindow* g_pdfPreviewWindow = nil;
static id g_pdfPreviewDelegate = nil;
static NSWindow* g_mainWindow = nil;

static WKWebView* findWKWebView(NSView* view);

static void setWKWebViewUserAgentAndMedia(void* nsWindowPtr, const char* uaStr) {
    @autoreleasepool {
        configureWebKitMemoryLimits();
        NSWindow* win = (__bridge NSWindow*)nsWindowPtr;
        g_mainWindow = win;
        NSView* contentView = [win contentView];
        WKWebView* wv = [contentView isKindOfClass:[WKWebView class]] ? (WKWebView*)contentView : findWKWebView(contentView);
        if (wv) {
            g_mainWebView = wv;
            [wv setAutoresizingMask:(NSViewWidthSizable | NSViewHeightSizable)];
            NSString* ua = [NSString stringWithUTF8String:uaStr];
            [wv setCustomUserAgent:ua];

            // Enable GPU-accelerated drawing & asynchronous layer rendering for smooth scrolling
            [wv setWantsLayer:YES];
            if (wv.layer) {
                [wv.layer setDrawsAsynchronously:YES];
                [wv.layer setOpaque:YES];
            }

            // Enable seamless inline video & audio playback for Status/Stories and media
            @try {
                [wv.configuration setMediaTypesRequiringUserActionForPlayback:WKAudiovisualMediaTypeNone];
                [wv.configuration setValue:@(WKAudiovisualMediaTypeNone) forKey:@"mediaTypesRequiringUserActionForPlayback"];
                [wv.configuration setValue:@YES forKey:@"allowsInlineMediaPlayback"];
                [wv.configuration setAllowsAirPlayForMediaPlayback:YES];
            } @catch (NSException *e) {}

            // Memory and Performance Optimizations
            @try {
                WKPreferences* prefs = [wv.configuration preferences];
                [prefs setValue:@YES forKey:@"webGLEnabled"];
                // developerExtrasEnabled is intentionally left off in production: it keeps
                // Web Inspector instrumentation resident in the WebContent process for the
                // lifetime of the app, adding avoidable CPU/RAM overhead with no user benefit.

                // Disable pageCache & backForwardCache to prevent WebKit from retaining old page trees
                [prefs setValue:@NO forKey:@"backForwardCacheEnabled"];
                [prefs setValue:@NO forKey:@"pageCacheEnabled"];
            } @catch (NSException *exception) {}

            g_uiDelegate = [[WhatsAppUIDelegate alloc] init];
            [wv setUIDelegate:g_uiDelegate];
        }
    }
}

static void configureWindowBehavior(void* nsWindowPtr) {
    @autoreleasepool {
        NSWindow* win = (__bridge NSWindow*)nsWindowPtr;
        g_mainWindow = win;
        NSView* cv = [win contentView];
        WKWebView* wv = [cv isKindOfClass:[WKWebView class]] ? (WKWebView*)cv : findWKWebView(cv);
        if (wv) {
            g_mainWebView = wv;
        }

        // Ensure window is fully resizable, minimizable, and supports fullscreen
        NSWindowStyleMask mask = [win styleMask];
        mask |= (NSWindowStyleMaskResizable | NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable);
        [win setStyleMask:mask];

        [win setCollectionBehavior:(NSWindowCollectionBehaviorFullScreenPrimary | NSWindowCollectionBehaviorDefault)];
        [win setShowsResizeIndicator:YES];

        // Minimum bounds: allow shrinking down dynamically to compact window
        [win setMinSize:NSMakeSize(450, 320)];
        [win setContentMinSize:NSMakeSize(450, 320)];
        [win setMaxSize:NSMakeSize(FLT_MAX, FLT_MAX)];
        [win setContentMaxSize:NSMakeSize(FLT_MAX, FLT_MAX)];

        win.appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
        win.titlebarAppearsTransparent = YES;

        NSView* contentView = [win contentView];
        if (contentView) {
            [contentView setAutoresizingMask:(NSViewWidthSizable | NSViewHeightSizable)];
        }

        g_windowDelegate = [[WhatsAppWindowDelegate alloc] init];
        [win setDelegate:g_windowDelegate];

        g_appDelegate = [[WhatsAppAppDelegate alloc] init];
        g_appDelegate.window = win;
        [NSApp setDelegate:g_appDelegate];
        [[NSUserNotificationCenter defaultUserNotificationCenter] setDelegate:g_appDelegate];
    }
}

static BOOL g_isAlwaysOnTop = NO;

static BOOL toggleAlwaysOnTop(void* nsWindowPtr) {
    @autoreleasepool {
        NSWindow* win = (__bridge NSWindow*)nsWindowPtr;
        g_isAlwaysOnTop = !g_isAlwaysOnTop;
        if (g_isAlwaysOnTop) {
            [win setLevel:NSFloatingWindowLevel];
        } else {
            [win setLevel:NSNormalWindowLevel];
        }
        return g_isAlwaysOnTop;
    }
}

static void setDockBadge(const char* labelStr) {
    @autoreleasepool {
        NSString* label = (labelStr && strlen(labelStr) > 0) ? [NSString stringWithUTF8String:labelStr] : nil;
        [[NSApp dockTile] setBadgeLabel:label];
    }
}

static void getWindowFrame(void* nsWindowPtr, double* x, double* y, double* w, double* h) {
    @autoreleasepool {
        NSWindow* win = (__bridge NSWindow*)nsWindowPtr;
        NSRect frame = [win frame];
        *x = (double)frame.origin.x;
        *y = (double)frame.origin.y;
        *w = (double)frame.size.width;
        *h = (double)frame.size.height;
    }
}

// Per-monitor window placement. macOS display IDs are stable across sessions,
// so each monitor remembers its own frame. The returned static buffer is
// valid only until the next call and must be used on the main thread.
static char g_screenIDBuf[32];

static const char* currentScreenIdentifier(void) {
    @autoreleasepool {
        NSScreen* screen = [NSScreen mainScreen];
        NSNumber* id = screen ? [screen deviceDescription][@"NSScreenNumber"] : nil;
        snprintf(g_screenIDBuf, sizeof(g_screenIDBuf), "%llu", id.unsignedLongLongValue);
        return g_screenIDBuf;
    }
}

static const char* windowScreenIdentifier(void* nsWindowPtr) {
    @autoreleasepool {
        NSWindow* win = (__bridge NSWindow*)nsWindowPtr;
        NSScreen* screen = nil;
        if (win) {
            NSRect frame = [win frame];
            NSPoint probe = NSMakePoint(frame.origin.x + 10, frame.origin.y + 10);
            for (NSScreen* s in [NSScreen screens]) {
                if (NSPointInRect(probe, [s frame])) { screen = s; break; }
            }
        }
        if (!screen) screen = [NSScreen mainScreen];
        NSNumber* id = screen ? [screen deviceDescription][@"NSScreenNumber"] : nil;
        snprintf(g_screenIDBuf, sizeof(g_screenIDBuf), "%llu", id.unsignedLongLongValue);
        return g_screenIDBuf;
    }
}

// A saved frame is restored only when a meaningful part of it still lies on a
// connected screen; otherwise the window would open invisible after a monitor
// was unplugged. Returns 1/0 as int: BOOL is bool on arm64 but signed char on
// x86_64, and cgo needs one Go type for the universal binary.
static int frameIntersectsAnyScreenInt(double x, double y, double w, double h) {
    @autoreleasepool {
        NSRect test = NSMakeRect(x, y, w, h);
        for (NSScreen* s in [NSScreen screens]) {
            NSRect inter = NSIntersectionRect(test, [s visibleFrame]);
            if (inter.size.width > 60 && inter.size.height > 40) return 1;
        }
        return 0;
    }
}

static void setWindowFrame(void* nsWindowPtr, double x, double y, double w, double h) {
    @autoreleasepool {
        NSWindow* win = (__bridge NSWindow*)nsWindowPtr;
        NSRect rect = NSMakeRect((CGFloat)x, (CGFloat)y, (CGFloat)w, (CGFloat)h);
        [win setFrame:rect display:YES animate:NO];
    }
}

static WKWebView* findWKWebView(NSView* view) {
    if (!view) return nil;
    if ([view isKindOfClass:[WKWebView class]]) {
        return (WKWebView*)view;
    }
    for (NSView* subview in [view subviews]) {
        WKWebView* found = findWKWebView(subview);
        if (found) return found;
    }
    return nil;
}

// Menu commands may run while the menu bar owns focus, so keyWindow/mainWindow
// can temporarily be nil. Keep the configured application window as the source
// of truth and only use AppKit's active-window lookup as a fallback.
static NSWindow* appWindow(void) {
    if (g_mainWindow) {
        return g_mainWindow;
    }
    if (g_appDelegate && g_appDelegate.window) {
        return g_appDelegate.window;
    }
    for (NSWindow* w in [NSApp windows]) {
        if (![w isKindOfClass:[NSPanel class]] && [w canBecomeKeyWindow]) {
            return w;
        }
    }
    return [NSApp keyWindow] ?: [NSApp mainWindow];
}

static WKWebView* appWebView(void) {
    if (g_mainWebView) {
        return g_mainWebView;
    }
    NSWindow* win = appWindow();
    if (!win) return nil;
    NSView* cv = [win contentView];
    WKWebView* wv = [cv isKindOfClass:[WKWebView class]] ? (WKWebView*)cv : findWKWebView(cv);
    if (wv) g_mainWebView = wv;
    return wv;
}

static void showAppWindow(void) {
    NSWindow* win = appWindow();
    if (win) {
        if ([win isMiniaturized]) {
            [win deminiaturize:nil];
        }
        [win makeKeyAndOrderFront:nil];
        [win setIsVisible:YES];
    }
    [NSApp activateIgnoringOtherApps:YES];
}

static void evaluateAppJavaScript(NSString* script) {
    dispatch_async(dispatch_get_main_queue(), ^{
        showAppWindow();
        WKWebView* wv = appWebView();
        if (wv) {
            NSString* wrapped = [NSString stringWithFormat:@"(function(){ try { %@ } catch(e){ console.error(e); } })(); void 0;", script];
            [wv evaluateJavaScript:wrapped completionHandler:^(id result, NSError *error) {
                if (error) {
                    NSLog(@"[WhatsApp Desk] JS evaluation error: %@ for script: %@", error, script);
                }
            }];
        } else {
            NSLog(@"[WhatsApp Desk] Error: WKWebView not found for script: %@", script);
        }
    });
}

@interface PDFPreviewWindowDelegate : NSObject <NSWindowDelegate>
@end

@implementation PDFPreviewWindowDelegate
- (void)windowWillClose:(NSNotification*)notification {
    NSWindow* window = (NSWindow*)[notification object];
    NSView* contentView = [window contentView];
    if ([contentView isKindOfClass:[PDFView class]]) {
        PDFView* pdfView = (PDFView*)contentView;
        [pdfView setDocument:nil];
    }
    [window setContentView:nil];
    purgeWebKitMemory();
    evaluateAppJavaScript(@"if (window.closeDocumentViewerAfterNativePreview) { window.closeDocumentViewerAfterNativePreview(); }");
}
@end

static void openDownloadsFolderNative(void) {
    @autoreleasepool {
        NSString *downloads = [NSSearchPathForDirectoriesInDomains(NSDownloadsDirectory, NSUserDomainMask, YES) firstObject];
        NSString *targetFolder = [downloads stringByAppendingPathComponent:@"WhatsApp Downloads"];

        NSString *support = [NSSearchPathForDirectoriesInDomains(NSApplicationSupportDirectory, NSUserDomainMask, YES) firstObject];
        NSString *settingsFile = [support stringByAppendingPathComponent:@"WhatsAppDesk/settings.json"];
        if ([[NSFileManager defaultManager] fileExistsAtPath:settingsFile]) {
            NSData *data = [NSData dataWithContentsOfFile:settingsFile];
            if (data) {
                NSDictionary *json = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
                if (json && [json objectForKey:@"download_dir"]) {
                    NSString *custom = [json objectForKey:@"download_dir"];
                    if ([custom length] > 0) targetFolder = custom;
                }
            }
        }

        BOOL isDir = NO;
        if (![[NSFileManager defaultManager] fileExistsAtPath:targetFolder isDirectory:&isDir]) {
            [[NSFileManager defaultManager] createDirectoryAtPath:targetFolder withIntermediateDirectories:YES attributes:nil error:nil];
        }

        [[NSWorkspace sharedWorkspace] openURL:[NSURL fileURLWithPath:targetFolder]];
        [[NSWorkspace sharedWorkspace] activateFileViewerSelectingURLs:@[[NSURL fileURLWithPath:targetFolder]]];
    }
}

static BOOL showNativePDFPreview(const char* pathStr) {
    if (!pathStr || strlen(pathStr) == 0) return NO;
    NSString* path = [NSString stringWithUTF8String:pathStr];
    if (![[NSFileManager defaultManager] fileExistsAtPath:path]) return NO;

    void (^showPreview)(void) = ^{
        @autoreleasepool {
            NSURL* url = [NSURL fileURLWithPath:path];
            PDFDocument* document = [[PDFDocument alloc] initWithURL:url];
            if (!document) return;

            if (!g_pdfPreviewWindow) {
                NSRect frame = NSMakeRect(0, 0, 920, 760);
                g_pdfPreviewWindow = [[NSWindow alloc]
                    initWithContentRect:frame
                    styleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
                               NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable)
                    backing:NSBackingStoreBuffered
                    defer:NO];
                [g_pdfPreviewWindow setReleasedWhenClosed:NO];
                g_pdfPreviewDelegate = [[PDFPreviewWindowDelegate alloc] init];
                [g_pdfPreviewWindow setDelegate:g_pdfPreviewDelegate];
                [g_pdfPreviewWindow center];
            }

            NSRect pdfFrame = [[g_pdfPreviewWindow contentView] bounds];
            if (NSIsEmptyRect(pdfFrame)) {
                NSRect contentRect = [g_pdfPreviewWindow contentRectForFrameRect:[g_pdfPreviewWindow frame]];
                pdfFrame = NSMakeRect(0, 0, NSWidth(contentRect), NSHeight(contentRect));
            }
            PDFView* pdfView = [[PDFView alloc] initWithFrame:pdfFrame];
            [pdfView setAutoresizingMask:(NSViewWidthSizable | NSViewHeightSizable)];
            [pdfView setAutoScales:YES];
            [pdfView setDisplayMode:kPDFDisplaySinglePageContinuous];
            [pdfView setDocument:document];
            [g_pdfPreviewWindow setContentView:pdfView];
#if !__has_feature(objc_arc)
            [pdfView release];
            [document release];
#endif
            [g_pdfPreviewWindow setTitle:[path lastPathComponent]];
            [g_pdfPreviewWindow makeKeyAndOrderFront:nil];
            [NSApp activateIgnoringOtherApps:YES];
        }
    };

    if ([NSThread isMainThread]) {
        showPreview();
    } else {
        dispatch_async(dispatch_get_main_queue(), showPreview);
    }
    return YES;
}

// Normalize Objective-C BOOL to C int so cgo has the same Go type on both
// arm64 and x86_64 macOS targets.
static int toggleAlwaysOnTopInt(void* nsWindowPtr) {
    return toggleAlwaysOnTop(nsWindowPtr) ? 1 : 0;
}

static int showNativePDFPreviewInt(const char* pathStr) {
    return showNativePDFPreview(pathStr) ? 1 : 0;
}

static void setNativeWindowTheme(void* nsWindowPtr, const char* themeStr) {
    @autoreleasepool {
        NSWindow* win = (__bridge NSWindow*)nsWindowPtr;
        if (!win) return;
        NSString* theme = [NSString stringWithUTF8String:themeStr];
        if ([theme isEqualToString:@"light"]) {
            win.appearance = [NSAppearance appearanceNamed:NSAppearanceNameAqua];
        } else if ([theme isEqualToString:@"dark"]) {
            win.appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
        } else {
            win.appearance = nil;
        }
    }
}

@interface MenuBridge : NSObject
- (void)menuSettings:(id)sender;
- (void)menuCheckUpdates:(id)sender;
- (void)menuOpenDownloads:(id)sender;
- (void)menuTogglePrivacy:(id)sender;
- (void)menuToggleAlwaysOnTop:(id)sender;
- (void)menuToggleMuteAudio:(id)sender;
- (void)menuReloadChat:(id)sender;
- (void)menuHardRefresh:(id)sender;
- (void)menuShowApp:(id)sender;
- (void)menuSetThemeDark:(id)sender;
- (void)menuSetThemeLight:(id)sender;
- (void)menuSetThemeSystem:(id)sender;
@end

@implementation MenuBridge
- (void)menuSettings:(id)sender {
    evaluateAppJavaScript(@"if (window.showSettingsModal) { window.showSettingsModal(); }");
}
- (void)menuCheckUpdates:(id)sender {
    evaluateAppJavaScript(@"if (window.triggerCheckForUpdate) { window.triggerCheckForUpdate(); }");
}
- (void)menuOpenDownloads:(id)sender {
    openDownloadsFolderNative();
}
- (void)menuTogglePrivacy:(id)sender {
    evaluateAppJavaScript(@"if (window.togglePrivacyMode) { window.togglePrivacyMode(); }");
}
- (void)menuToggleAlwaysOnTop:(id)sender {
    NSWindow* win = appWindow();
    if (win) {
        toggleAlwaysOnTop((__bridge void*)win);
    }
    evaluateAppJavaScript(@"if (window.updateBadges) { window.updateBadges(); }");
}
- (void)menuToggleMuteAudio:(id)sender {
    evaluateAppJavaScript(@"if (window.toggleMuteAudio) { window.toggleMuteAudio(); }");
}
- (void)menuReloadChat:(id)sender {
    dispatch_async(dispatch_get_main_queue(), ^{
        showAppWindow();
        WKWebView* wv = appWebView();
        if (wv) {
            [wv reload];
        } else {
            evaluateAppJavaScript(@"window.location.reload();");
        }
    });
}
- (void)menuHardRefresh:(id)sender {
    dispatch_async(dispatch_get_main_queue(), ^{
        purgeWebKitMemory();
        showAppWindow();
        WKWebView* wv = appWebView();
        if (wv) {
            [wv reloadFromOrigin];
        } else {
            evaluateAppJavaScript(@"window.location.href = window.location.origin + window.location.pathname + '?_t=' + Date.now();");
        }
    });
}
- (void)menuShowApp:(id)sender {
    dispatch_async(dispatch_get_main_queue(), ^{
        showAppWindow();
    });
}
- (void)menuSetThemeDark:(id)sender {
    NSWindow* win = appWindow();
    if (win) setNativeWindowTheme((__bridge void*)win, "dark");
    evaluateAppJavaScript(@"if (window.setAppTheme) { window.setAppTheme('dark'); }");
}
- (void)menuSetThemeLight:(id)sender {
    NSWindow* win = appWindow();
    if (win) setNativeWindowTheme((__bridge void*)win, "light");
    evaluateAppJavaScript(@"if (window.setAppTheme) { window.setAppTheme('light'); }");
}
- (void)menuSetThemeSystem:(id)sender {
    NSWindow* win = appWindow();
    if (win) setNativeWindowTheme((__bridge void*)win, "system");
    evaluateAppJavaScript(@"if (window.setAppTheme) { window.setAppTheme('system'); }");
}
@end

static MenuBridge* g_menuBridge = nil;
static NSStatusItem* g_statusItem = nil;

static void setupStatusItem(void) {
    @autoreleasepool {
        if (g_statusItem) return;
        if (!g_menuBridge) {
            g_menuBridge = [[MenuBridge alloc] init];
        }

        NSStatusBar* statusBar = [NSStatusBar systemStatusBar];
        g_statusItem = [statusBar statusItemWithLength:NSSquareStatusItemLength];

        NSStatusBarButton* button = [g_statusItem button];
        if (button) {
            NSImage* icon = nil;
            if (@available(macOS 11.0, *)) {
                icon = [NSImage imageWithSystemSymbolName:@"message.fill" accessibilityDescription:@"WhatsApp"];
            }
            if (!icon) {
                // Fallback to app icon, but make it a template image for Dark mode
                icon = [NSApp applicationIconImage];
                if (icon) {
                    // Create a template version by copying and setting template flag
                    NSImage* templateIcon = [[NSImage alloc] initWithSize:icon.size];
                    [templateIcon lockFocus];
                    [icon drawAtPoint:NSZeroPoint fromRect:NSZeroRect operation:NSCompositingOperationSourceOver fraction:1.0];
                    [templateIcon unlockFocus];
                    [templateIcon setTemplate:YES];
                    icon = templateIcon;
                }
            }
            if (icon) {
                [icon setTemplate:YES];
                [icon setSize:NSMakeSize(18, 18)];
                [button setImage:icon];
            }
            [button setToolTip:@"WhatsApp Desktop"];
        }

        NSMenu* trayMenu = [[NSMenu alloc] initWithTitle:@"WhatsApp Tray"];

        NSMenuItem* appTitle = [trayMenu addItemWithTitle:@"WhatsApp Desk" action:nil keyEquivalent:@""];
        [appTitle setEnabled:NO];

        [trayMenu addItem:[NSMenuItem separatorItem]];

        NSMenuItem* openItem = [trayMenu addItemWithTitle:@"Show Window" action:@selector(menuShowApp:) keyEquivalent:@""];
        [openItem setTarget:g_menuBridge];

        NSMenuItem* settingsItem = [trayMenu addItemWithTitle:@"Control Center & Settings..." action:@selector(menuSettings:) keyEquivalent:@","];
        [settingsItem setTarget:g_menuBridge];

        [trayMenu addItem:[NSMenuItem separatorItem]];

        // Submenu: Theme
        NSMenuItem* themeSubmenuItem = [[NSMenuItem alloc] initWithTitle:@"Appearance Theme" action:nil keyEquivalent:@""];
        NSMenu* themeMenu = [[NSMenu alloc] initWithTitle:@"Appearance Theme"];

        NSMenuItem* mDark = [themeMenu addItemWithTitle:@"🌙 Dark Mode" action:@selector(menuSetThemeDark:) keyEquivalent:@""];
        [mDark setTarget:g_menuBridge];

        NSMenuItem* mLight = [themeMenu addItemWithTitle:@"☀️ Light Mode" action:@selector(menuSetThemeLight:) keyEquivalent:@""];
        [mLight setTarget:g_menuBridge];

        NSMenuItem* mSystem = [themeMenu addItemWithTitle:@"💻 Follow System (Auto)" action:@selector(menuSetThemeSystem:) keyEquivalent:@""];
        [mSystem setTarget:g_menuBridge];

        [themeSubmenuItem setSubmenu:themeMenu];
        [trayMenu addItem:themeSubmenuItem];

        [trayMenu addItem:[NSMenuItem separatorItem]];

        NSMenuItem* privItem = [trayMenu addItemWithTitle:@"Toggle Privacy Mode" action:@selector(menuTogglePrivacy:) keyEquivalent:@""];
        [privItem setTarget:g_menuBridge];

        NSMenuItem* topItem = [trayMenu addItemWithTitle:@"Toggle Always on Top" action:@selector(menuToggleAlwaysOnTop:) keyEquivalent:@""];
        [topItem setTarget:g_menuBridge];

        NSMenuItem* muteItem = [trayMenu addItemWithTitle:@"Toggle Mute Audio" action:@selector(menuToggleMuteAudio:) keyEquivalent:@""];
        [muteItem setTarget:g_menuBridge];

        NSMenuItem* dlItem = [trayMenu addItemWithTitle:@"Open Downloads Folder" action:@selector(menuOpenDownloads:) keyEquivalent:@""];
        [dlItem setTarget:g_menuBridge];

        NSMenuItem* updItem = [trayMenu addItemWithTitle:@"Check for Updates..." action:@selector(menuCheckUpdates:) keyEquivalent:@""];
        [updItem setTarget:g_menuBridge];

        [trayMenu addItem:[NSMenuItem separatorItem]];

        NSMenuItem* relItem = [trayMenu addItemWithTitle:@"Reload Chat" action:@selector(menuReloadChat:) keyEquivalent:@""];
        [relItem setTarget:g_menuBridge];

        [trayMenu addItemWithTitle:@"Quit WhatsApp Desk" action:@selector(terminate:) keyEquivalent:@"q"];

        [g_statusItem setMenu:trayMenu];
    }
}

static void setupMacOSMenuBar(void) {
    @autoreleasepool {
        if (!g_menuBridge) {
            g_menuBridge = [[MenuBridge alloc] init];
        }

        NSMenu* mainMenu = [[NSMenu alloc] init];

        // App Menu
        NSMenuItem* appMenuItem = [[NSMenuItem alloc] init];
        NSMenu* appMenu = [[NSMenu alloc] initWithTitle:@"WhatsApp"];
        NSMenuItem* aboutItem = [appMenu addItemWithTitle:@"About WhatsApp Desk" action:@selector(orderFrontStandardAboutPanel:) keyEquivalent:@""];
        [aboutItem setTarget:NSApp];

        // NOTE: no key equivalents here on purpose. Every shortcut is owned
        // by the page script (with toast feedback); a native equivalent for
        // the same keys would fire the action TWICE (menu + page).
        NSMenuItem* settingsItem = [appMenu addItemWithTitle:@"Settings..." action:@selector(menuSettings:) keyEquivalent:@""];
        [settingsItem setTarget:g_menuBridge];

        NSMenuItem* updateItem = [appMenu addItemWithTitle:@"Check for Updates..." action:@selector(menuCheckUpdates:) keyEquivalent:@""];
        [updateItem setTarget:g_menuBridge];

        NSMenuItem* dlItem = [appMenu addItemWithTitle:@"Open Downloads Folder" action:@selector(menuOpenDownloads:) keyEquivalent:@""];
        [dlItem setTarget:g_menuBridge];

        [appMenu addItem:[NSMenuItem separatorItem]];
        [appMenu addItemWithTitle:@"Hide WhatsApp Desk" action:@selector(hide:) keyEquivalent:@"h"];
        NSMenuItem* hideOthers = [appMenu addItemWithTitle:@"Hide Others" action:@selector(hideOtherApplications:) keyEquivalent:@"h"];
        [hideOthers setKeyEquivalentModifierMask:(NSEventModifierFlagOption | NSEventModifierFlagCommand)];
        [appMenu addItemWithTitle:@"Show All" action:@selector(unhideAllApplications:) keyEquivalent:@""];
        [appMenu addItem:[NSMenuItem separatorItem]];
        [appMenu addItemWithTitle:@"Quit WhatsApp Desk" action:@selector(terminate:) keyEquivalent:@"q"];
        [appMenuItem setSubmenu:appMenu];
        [mainMenu addItem:appMenuItem];

        // Edit Menu (Essential for Cmd+C, Cmd+V, Cmd+X, Cmd+A, Cmd+Z)
        NSMenuItem* editMenuItem = [[NSMenuItem alloc] init];
        NSMenu* editMenu = [[NSMenu alloc] initWithTitle:@"Edit"];
        [editMenu addItemWithTitle:@"Undo" action:@selector(undo:) keyEquivalent:@"z"];
        NSMenuItem* redo = [editMenu addItemWithTitle:@"Redo" action:@selector(redo:) keyEquivalent:@"Z"];
        [redo setKeyEquivalentModifierMask:(NSEventModifierFlagShift | NSEventModifierFlagCommand)];
        [editMenu addItem:[NSMenuItem separatorItem]];
        [editMenu addItemWithTitle:@"Cut" action:@selector(cut:) keyEquivalent:@"x"];
        [editMenu addItemWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
        [editMenu addItemWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"];
        [editMenu addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];
        [editMenuItem setSubmenu:editMenu];
        [mainMenu addItem:editMenuItem];

        // Controls Menu
        NSMenuItem* controlsMenuItem = [[NSMenuItem alloc] init];
        NSMenu* controlsMenu = [[NSMenu alloc] initWithTitle:@"Controls"];

        // Submenu: Theme in Controls menu
        NSMenuItem* themeSubItem = [[NSMenuItem alloc] initWithTitle:@"Theme" action:nil keyEquivalent:@""];
        NSMenu* subTheme = [[NSMenu alloc] initWithTitle:@"Theme"];
        NSMenuItem* thDark = [subTheme addItemWithTitle:@"Dark" action:@selector(menuSetThemeDark:) keyEquivalent:@""];
        [thDark setTarget:g_menuBridge];
        NSMenuItem* thLight = [subTheme addItemWithTitle:@"Light" action:@selector(menuSetThemeLight:) keyEquivalent:@""];
        [thLight setTarget:g_menuBridge];
        NSMenuItem* thSystem = [subTheme addItemWithTitle:@"System Default" action:@selector(menuSetThemeSystem:) keyEquivalent:@""];
        [thSystem setTarget:g_menuBridge];
        [themeSubItem setSubmenu:subTheme];
        [controlsMenu addItem:themeSubItem];

        [controlsMenu addItem:[NSMenuItem separatorItem]];

        // NOTE: shortcuts live in the page script only (see note above).
        NSMenuItem* privItem = [controlsMenu addItemWithTitle:@"Toggle Privacy Mode" action:@selector(menuTogglePrivacy:) keyEquivalent:@""];
        [privItem setTarget:g_menuBridge];

        NSMenuItem* topItem = [controlsMenu addItemWithTitle:@"Toggle Always on Top" action:@selector(menuToggleAlwaysOnTop:) keyEquivalent:@""];
        [topItem setTarget:g_menuBridge];

        NSMenuItem* muteItem = [controlsMenu addItemWithTitle:@"Toggle Audio Mute" action:@selector(menuToggleMuteAudio:) keyEquivalent:@""];
        [muteItem setTarget:g_menuBridge];

        [controlsMenu addItem:[NSMenuItem separatorItem]];

        NSMenuItem* relItem = [controlsMenu addItemWithTitle:@"Reload Chat" action:@selector(menuReloadChat:) keyEquivalent:@""];
        [relItem setTarget:g_menuBridge];

        NSMenuItem* hardRelItem = [controlsMenu addItemWithTitle:@"Hard Refresh (Clear Cache)" action:@selector(menuHardRefresh:) keyEquivalent:@""];
        [hardRelItem setTarget:g_menuBridge];

        [controlsMenu addItem:[NSMenuItem separatorItem]];

        NSMenuItem* menuOpenFolder = [controlsMenu addItemWithTitle:@"Open Downloads Folder" action:@selector(menuOpenDownloads:) keyEquivalent:@""];
        [menuOpenFolder setTarget:g_menuBridge];

        NSMenuItem* menuCheckUpdates = [controlsMenu addItemWithTitle:@"Check for Updates..." action:@selector(menuCheckUpdates:) keyEquivalent:@""];
        [menuCheckUpdates setTarget:g_menuBridge];

        NSMenuItem* menuSettingsDialog = [controlsMenu addItemWithTitle:@"Settings / Control Center..." action:@selector(menuSettings:) keyEquivalent:@","];
        [menuSettingsDialog setTarget:g_menuBridge];

        [controlsMenuItem setSubmenu:controlsMenu];
        [mainMenu addItem:controlsMenuItem];

        // Window Menu
        NSMenuItem* windowMenuItem = [[NSMenuItem alloc] init];
        NSMenu* windowMenu = [[NSMenu alloc] initWithTitle:@"Window"];
        [windowMenu addItemWithTitle:@"Minimize" action:@selector(performMiniaturize:) keyEquivalent:@"m"];
        [windowMenu addItemWithTitle:@"Zoom" action:@selector(performZoom:) keyEquivalent:@""];
        [windowMenu addItem:[NSMenuItem separatorItem]];
        [windowMenu addItemWithTitle:@"Close Window" action:@selector(performClose:) keyEquivalent:@"w"];
        [windowMenuItem setSubmenu:windowMenu];
        [mainMenu addItem:windowMenuItem];

        [NSApp setMainMenu:mainMenu];
        [NSApp setWindowsMenu:windowMenu];
    }
}
*/
import "C"
import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	webview "github.com/webview/webview_go"
)

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"

func getUserDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	dir := filepath.Join(home, "Library", "Application Support", "WhatsAppDesk", "UserData")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// loadWindowStateForScreen returns the frame saved for the given display ID,
// falling back to the legacy single-frame state. A frame is rejected when it
// no longer intersects any connected screen, so unplugging a monitor never
// leaves the window stranded off-screen.
func loadWindowStateForScreen(dir, screenID string) *WindowState {
	path := filepath.Join(dir, "window_state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var state WindowState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil
	}
	candidates := []*WindowState{}
	if screenID != "" {
		if per, ok := state.Screens[screenID]; ok && per.Width >= 450 && per.Height >= 320 {
			candidates = append(candidates, &per)
		}
	}
	if state.Width >= 450 && state.Height >= 320 {
		fallback := state
		fallback.Screens = nil
		candidates = append(candidates, &fallback)
	}
	for _, cand := range candidates {
		if C.frameIntersectsAnyScreenInt(C.double(cand.X), C.double(cand.Y), C.double(cand.Width), C.double(cand.Height)) != 0 {
			return cand
		}
	}
	return nil
}

func saveWindowState(dir string, win unsafe.Pointer) {
	if win == nil {
		return
	}
	var x, y, w, h C.double
	C.getWindowFrame(win, &x, &y, &w, &h)
	if w < 450 || h < 320 {
		return
	}
	screenID := C.GoString(C.windowScreenIdentifier(win))

	// Read-modify-write the whole file so frames of other monitors survive.
	state := WindowState{
		X: float64(x), Y: float64(y), Width: float64(w), Height: float64(h),
	}
	if data, err := os.ReadFile(filepath.Join(dir, "window_state.json")); err == nil {
		var existing WindowState
		if json.Unmarshal(data, &existing) == nil {
			state.Screens = existing.Screens
		}
	}
	if state.Screens == nil {
		state.Screens = map[string]WindowState{}
	}
	if prev, ok := state.Screens[screenID]; ok &&
		prev.X == state.X && prev.Y == state.Y &&
		prev.Width == state.Width && prev.Height == state.Height {
		return // nothing changed on this monitor — skip the disk write
	}
	state.Screens[screenID] = WindowState{X: state.X, Y: state.Y, Width: state.Width, Height: state.Height}
	data, err := json.MarshalIndent(state, "", "  ")
	if err == nil {
		_ = os.WriteFile(filepath.Join(dir, "window_state.json"), data, 0644)
	}
}

func checkSingleInstance() (*os.File, bool) {
	dir := getUserDataDir()
	lockPath := filepath.Join(dir, "whatsapp.lock")
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, true
	}
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		_ = exec.Command("osascript", "-e", `tell application "WhatsApp Desk" to activate`).Run()
		return nil, false
	}
	return file, true
}

func getAppBundlePath() string {
	execPath, err := os.Executable()
	if err != nil {
		return "/Applications/WhatsApp Desk.app"
	}
	if idx := strings.Index(execPath, ".app"); idx != -1 {
		return execPath[:idx+4]
	}
	if _, err := os.Stat("/Applications/WhatsApp Desk.app"); err == nil {
		return "/Applications/WhatsApp Desk.app"
	}
	return execPath
}

func getLaunchAgentPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "LaunchAgents", "com.whatsapp.desk.plist")
}

func toggleAutoStartMac() bool {
	plistPath := getLaunchAgentPath()
	if plistPath == "" {
		return false
	}
	if _, err := os.Stat(plistPath); err == nil {
		_ = os.Remove(plistPath)
		return false
	}

	appPath := getAppBundlePath()
	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.whatsapp.desk</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/bin/open</string>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
</dict>
</plist>`, appPath)

	_ = os.MkdirAll(filepath.Dir(plistPath), 0755)
	if err := os.WriteFile(plistPath, []byte(plistContent), 0644); err != nil {
		return false
	}
	return true
}

func showNativeNotification(title, message string) {
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))
	cMsg := C.CString(message)
	defer C.free(unsafe.Pointer(cMsg))
	C.postNativeMacNotification(cTitle, cMsg)
}

func runApp() {
	lockFile, isSingle := checkSingleInstance()
	if !isSingle {
		os.Exit(0)
	}
	if lockFile != nil {
		defer lockFile.Close()
	}

	userDataDir := getUserDataDir()
	cacheDebugLog("startup: pid=%d profile=%s", os.Getpid(), userDataDir)

	w := webview.New(false)
	if w == nil {
		log.Fatalln("Gagal inisialisasi WebKit WebView")
	}
	defer w.Destroy()

	// 1. Configure window behavior: dark title bar, close-to-hide, and dock click reopen
	C.configureWindowBehavior(w.Window())

	// Apply configured appearance theme (dark / light / system)
	initSettings := loadSettings()
	cTheme := C.CString(initSettings.Theme)
	C.setNativeWindowTheme(w.Window(), cTheme)
	C.free(unsafe.Pointer(cTheme))

	// 2. Set native WebKit customUserAgent to Google Chrome & auto-grant media capture
	cua := C.CString(userAgent)
	C.setWKWebViewUserAgentAndMedia(w.Window(), cua)
	C.free(unsafe.Pointer(cua))

	// 3. Setup standard macOS menu bar and system status item (taskbar tray icon)
	C.setupMacOSMenuBar()
	C.setupStatusItem()

	w.SetTitle(windowTitle)

	// 4. Ensure window is initialized with HintNone (resizable), then restore
	// the frame saved for the monitor the window currently sits on.
	w.SetSize(windowWidth, windowHeight, webview.HintNone)
	screenID := C.GoString(C.windowScreenIdentifier(w.Window()))
	if state := loadWindowStateForScreen(userDataDir, screenID); state != nil {
		C.setWindowFrame(w.Window(), C.double(state.X), C.double(state.Y), C.double(state.Width), C.double(state.Height))
	}

	// 5. Save window state only after a debounced resize event from the page.
	// The callback runs on the WebView UI thread, as required by AppKit.
	_ = w.Bind("saveWindowStateNative", func(width, height int) {
		saveWindowState(userDataDir, w.Window())
	})

	// 6. Bind native notification bridge
	_ = w.Bind("sendNativeNotification", func(title, body string) {
		go showNativeNotification(title, body)
	})

	_ = w.Bind("releaseMemoryNative", func() {
		C.triggerNativeMemoryPurge()
		debugLogProcessStats("release-memory")
		if debugEnabled() {
			cacheDebugLog("cache disk total: %.0f MB", float64(C.whatsappDeskDiskCacheBytesSync())/1024/1024)
		}
	})

	// 7. Bind external link handler to open links in macOS default browser
	_ = w.Bind("openExternalLink", func(rawURL string) {
		if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
			_ = exec.Command("open", rawURL).Start()
		}
	})

	// 8. Bind dock badge unread counter
	_ = w.Bind("updateDockBadge", func(badge string) {
		cstr := C.CString(badge)
		defer C.free(unsafe.Pointer(cstr))
		C.setDockBadge(cstr)
	})

	// 9. Bind Always on Top toggle
	_ = w.Bind("toggleAlwaysOnTopNative", func() bool {
		return C.toggleAlwaysOnTopInt(w.Window()) != 0
	})

	// 10. Bind Auto-Start toggle
	_ = w.Bind("toggleAutoStartNative", func() bool {
		return toggleAutoStartMac()
	})

	// 11. Bind in-app auto updater
	_ = w.Bind("checkForUpdateNative", func(manual bool) UpdateInfo {
		info, err := checkForUpdate(appVersion)
		if err != nil {
			return UpdateInfo{CurrentVersion: appVersion, CheckError: err.Error()}
		}
		return *info
	})

	_ = w.Bind("startUpdateNative", func(downloadURL string) {
		go func() {
			_ = executeUpdate(w, downloadURL)
		}()
	})

	// 13. Bind download, preview, and settings handlers
	_ = w.Bind("saveDownloadedFileNative", func(filename, dataURI string) string {
		path, err := saveDownloadedFile(filename, dataURI)
		if err != nil {
			return ""
		}
		return path
	})

	_ = w.Bind("previewDocumentNative", func(filename, dataURI string) string {
		path, err := previewDocument(filename, dataURI)
		if err != nil {
			return ""
		}
		return path
	})

	_ = w.Bind("openFileNative", func(filePath string) bool {
		return openFileInDefaultApp(filePath)
	})

	_ = w.Bind("showPDFPreviewNative", func(filePath string) bool {
		path := C.CString(filePath)
		defer C.free(unsafe.Pointer(path))
		return C.showNativePDFPreviewInt(path) != 0
	})

	// Lazy-load SheetJS library for spreadsheet preview
	_ = w.Bind("loadXLSXLibraryNative", func() string {
		return xlsxLibJS
	})

	_ = w.Bind("getDownloadDirNative", func() string {
		s := loadSettings()
		return s.DownloadDir
	})

	_ = w.Bind("getOrganizeByMonthNative", func() bool {
		return loadSettings().OrganizeByMonth
	})

	_ = w.Bind("setOrganizeByMonthNative", func(on bool) bool {
		return setOrganizeByMonth(on)
	})

	_ = w.Bind("checkFileExistsNative", func(filename string) bool {
		return fileExistsInDownloadDir(filename)
	})

	_ = w.Bind("chooseDownloadDirNative", func() string {
		selected, err := chooseFolderDialog()
		if err != nil || selected == "" {
			return ""
		}
		s := loadSettings()
		s.DownloadDir = selected
		_ = saveSettings(s)
		return selected
	})

	_ = w.Bind("openDownloadDirNative", func() bool {
		s := loadSettings()
		_ = openFolderInFileManager(s.DownloadDir)
		return true
	})

	_ = w.Bind("resetDownloadDirNative", func() string {
		s := loadSettings()
		s.DownloadDir = getDefaultDownloadDir()
		_ = saveSettings(s)
		return s.DownloadDir
	})

	_ = w.Bind("getAppThemeNative", func() string {
		s := loadSettings()
		return s.Theme
	})

	_ = w.Bind("setAppThemeNative", func(theme string) string {
		saved := saveTheme(theme)
		cstr := C.CString(saved)
		defer C.free(unsafe.Pointer(cstr))
		C.setNativeWindowTheme(w.Window(), cstr)
		return saved
	})

	// Spell check bindings
	_ = w.Bind("getSpellCheckEnabledNative", func() bool {
		return getSpellCheckEnabled()
	})
	_ = w.Bind("setSpellCheckEnabledNative", func(enabled bool) bool {
		return setSpellCheckEnabled(enabled)
	})
	_ = w.Bind("getSpellCheckLangNative", func() string {
		return getSpellCheckLang()
	})
	_ = w.Bind("setSpellCheckLangNative", func(lang string) string {
		return setSpellCheckLang(lang)
	})
	_ = w.Bind("getBlurAvatarsNative", func() bool {
		return getBlurAvatars()
	})
	_ = w.Bind("setBlurAvatarsNative", func(on bool) bool {
		return setBlurAvatars(on)
	})
	_ = w.Bind("getPendingCrashNative", func() string {
		return pendingCrashReport()
	})
	_ = w.Bind("markCrashNotifiedNative", func() bool {
		return markCrashNotified()
	})

	w.Init(getInitScript(userAgent))
	w.Navigate(appURL)
	debugLogProcessStats("after-navigate")

	// 12. Check for updates in the background after startup & periodically
	go guardGoroutine("update-ticker", func() {
		checkAndNotifyUpdate := func() {
			info, err := checkForUpdate(appVersion)
			if err == nil && info != nil && info.Available {
				w.Dispatch(func() {
					script := fmt.Sprintf("if (window.showUpdateBanner) { window.showUpdateBanner(%q, %q, %q); }",
						info.LatestVersion, info.ReleaseTitle, info.DownloadURL)
					w.Eval(script)
				})
			}
		}

		time.Sleep(5 * time.Second)
		checkAndNotifyUpdate()

		ticker := time.NewTicker(4 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			checkAndNotifyUpdate()
		}
	})

	defer saveWindowState(userDataDir, w.Window())
	w.Run()
}
