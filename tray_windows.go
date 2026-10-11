//go:build windows

package main

// Windows tray icon and close-to-tray.
//
// macOS has had an NSStatusItem and Linux a StatusNotifierItem for a while;
// Windows had nothing, so closing the window ended the process and notifications
// stopped with it. This adds the missing tray icon, a Show/Hide/Quit menu, and
// makes the close button hide the window instead of ending the app.
//
// Everything here is best-effort and fail-open: if the tray cannot be created
// the window is left completely alone and the close button quits exactly as it
// did before. That matters because this code cannot be exercised by the test
// suite — a tray that fails must never be the reason the app is unusable.

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	trayClassName = "WhatsAppDeskTrayWindow"
	trayTooltip   = "WhatsApp Desk"

	trayWmApp          = 0x8000
	trayWmCallback     = trayWmApp + 1
	trayWmCommand      = 0x0111
	trayWmDestroy      = 0x0002
	trayWmNull         = 0x0000
	trayWmSysCommand   = 0x0112
	trayWmLButtonUp    = 0x0202
	trayWmRButtonUp    = 0x0205
	trayWmLButtonDbl   = 0x0203
	trayScClose        = 0xF060
	traySwHide         = 0
	traySwRestore      = 9
	trayGwlpWndProc    = ^uintptr(15) // GWLP_WNDPROC (-16)
	trayHwndMessage    = ^uintptr(2)  // HWND_MESSAGE (-3)
	trayNimAdd         = 0
	trayNimDelete      = 2
	trayNifMessage     = 0x00000001
	trayNifIcon        = 0x00000002
	trayNifTip         = 0x00000004
	trayMfString       = 0x00000000
	trayMfGrayed       = 0x00000001
	trayMfSeparator    = 0x00000800
	trayTpmRightButton = 0x0002
	trayTpmBottomAlign = 0x0020
	trayImageIcon      = 1
	trayLrDefaultSize  = 0x00000040
	trayLrShared       = 0x00008000
	trayIdiApplication = 32512
	trayIconResource   = 2 // the app icon resource the window already uses

	trayCmdShow = 1
	trayCmdHide = 2
	trayCmdQuit = 3
)

var (
	procTrayShellNotifyIcon  = windows.NewLazySystemDLL("shell32.dll").NewProc("Shell_NotifyIconW")
	procTrayGetCursorPos     = user32.NewProc("GetCursorPos")
	procTrayCreatePopupMenu  = user32.NewProc("CreatePopupMenu")
	procTrayAppendMenu       = user32.NewProc("AppendMenuW")
	procTrayTrackPopupMenu   = user32.NewProc("TrackPopupMenu")
	procTrayDestroyMenu      = user32.NewProc("DestroyMenu")
	procTrayPostMessage      = user32.NewProc("PostMessageW")
	procTrayGetModuleHandle  = kernel32.NewProc("GetModuleHandleW")
	procTrayLoadImage        = user32.NewProc("LoadImageW")
	procTrayRegisterClassEx  = user32.NewProc("RegisterClassExW")
	procTrayCreateWindowEx   = user32.NewProc("CreateWindowExW")
	procTrayDefWindowProc    = user32.NewProc("DefWindowProcW")
	procTraySetWindowLongPtr = user32.NewProc("SetWindowLongPtrW")
	procTrayCallWindowProc   = user32.NewProc("CallWindowProcW")
	procTrayIsWindowVisible  = user32.NewProc("IsWindowVisible")
)

// notifyIconData is the classic NOTIFYICONDATAW layout, cut off after szTip.
// Only NIF_MESSAGE, NIF_ICON and NIF_TIP are used, so the shorter cbSize is
// accepted and none of the later union members (which differ between shell
// versions) have to be guessed right.
type notifyIconData struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
}

type trayWndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

// Menu labels are package-level so the pointers handed to Win32 stay valid for
// the life of the process: a pointer into a temporary Go string would be freed
// by the collector while the shell still holds it.
var (
	trayLabelShow, _ = windows.UTF16FromString("Show WhatsApp Desk")
	trayLabelHide, _ = windows.UTF16FromString("Hide to tray")
	trayLabelQuit, _ = windows.UTF16FromString("Quit WhatsApp Desk")
)

var (
	trayMsgHWND      uintptr
	trayMainHWND     uintptr
	trayQuitFn       func()
	trayIconPath     string
	trayExePath      string
	trayInstalled    bool
	trayToldUser     bool
	trayOriginalProc uintptr
	traySubclassHWND uintptr

	// The callbacks are held here on purpose: the OS stores only the raw
	// address, so a callback that is not reachable from Go could be collected
	// while Windows still calls it.
	trayWndProcCallback  = syscall.NewCallback(trayWindowProc)
	trayMainProcCallback = syscall.NewCallback(trayMainWindowProc)
)

// trayAttach points the tray at the window it should drive, creating the tray
// on first use. An account switch rebuilds the window, so the target is
// refreshed while the icon itself stays put.
func trayAttach(hwnd uintptr, quit func(), iconPath, exePath string) {
	if !trayInstalled {
		if !trayCreate() {
			// No tray means no way back from a hidden window, so the close
			// button is left alone and keeps quitting the app as before.
			return
		}
		trayInstalled = true
	}
	trayMainHWND = hwnd
	trayQuitFn = quit
	trayIconPath = iconPath
	trayExePath = exePath
	traySubclassMainWindow(hwnd)
}

// trayDetach removes the icon on the way out, so no ghost icon is left behind
// for the user to hover over.
func trayDetach() {
	if !trayInstalled {
		return
	}
	var nid notifyIconData
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = trayMsgHWND
	nid.UID = 1
	_, _, _ = procTrayShellNotifyIcon.Call(trayNimDelete, uintptr(unsafe.Pointer(&nid)))
	trayInstalled = false
}

func trayCreate() bool {
	hInstance, _, _ := procTrayGetModuleHandle.Call(0)
	if hInstance == 0 {
		cacheDebugLog("tray: no module handle; leaving the close button alone")
		return false
	}
	className, err := windows.UTF16PtrFromString(trayClassName)
	if err != nil {
		return false
	}
	wc := trayWndClassEx{
		CbSize:        uint32(unsafe.Sizeof(trayWndClassEx{})),
		LpfnWndProc:   trayWndProcCallback,
		HInstance:     hInstance,
		LpszClassName: className,
	}
	// A non-zero return means the class was registered. Zero is tolerated: the
	// class may already exist, and CreateWindowExW below is the real test.
	if ret, _, _ := procTrayRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); ret == 0 {
		cacheDebugLog("tray: RegisterClassExW returned 0 (class may already exist)")
	}

	hwnd, _, errno := procTrayCreateWindowEx.Call(
		0,                                  // dwExStyle
		uintptr(unsafe.Pointer(className)), // lpClassName
		uintptr(unsafe.Pointer(className)), // lpWindowName
		0,                                  // dwStyle
		0, 0, 0, 0,                         // x, y, width, height
		trayHwndMessage, // hWndParent: message-only window
		0,               // hMenu
		hInstance,       // hInstance
		0,               // lpParam
	)
	if hwnd == 0 {
		cacheDebugLog("tray: CreateWindowExW failed: %v", errno)
		return false
	}
	trayMsgHWND = hwnd

	if !trayAddIcon(hInstance) {
		return false
	}
	cacheDebugLog("tray: installed")
	return true
}

func trayAddIcon(hInstance uintptr) bool {
	icon, _, _ := procTrayLoadImage.Call(hInstance, trayIconResource, trayImageIcon, 0, 0, trayLrDefaultSize|trayLrShared)
	if icon == 0 {
		icon, _, _ = procTrayLoadImage.Call(0, trayIdiApplication, trayImageIcon, 0, 0, trayLrDefaultSize|trayLrShared)
	}

	var nid notifyIconData
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = trayMsgHWND
	nid.UID = 1
	nid.UFlags = trayNifMessage | trayNifIcon | trayNifTip
	nid.UCallbackMessage = trayWmCallback
	nid.HIcon = icon
	tip := windows.StringToUTF16(trayTooltip)
	copy(nid.SzTip[:], tip)

	ret, _, errno := procTrayShellNotifyIcon.Call(trayNimAdd, uintptr(unsafe.Pointer(&nid)))
	if ret == 0 {
		cacheDebugLog("tray: Shell_NotifyIcon(NIM_ADD) failed: %v", errno)
		return false
	}
	return true
}

// traySubclassMainWindow routes the window's own messages through
// trayMainWindowProc so the close button can hide the window.
//
// It deliberately does not touch WM_CLOSE: the app itself closes the window
// with a posted WM_CLOSE during an account switch, and swallowing that would
// leave the old window alive next to the new one. Only the user's own close
// request arrives as WM_SYSCOMMAND/SC_CLOSE.
func traySubclassMainWindow(hwnd uintptr) {
	if hwnd == 0 || hwnd == traySubclassHWND {
		return
	}
	prev, _, errno := procTraySetWindowLongPtr.Call(hwnd, trayGwlpWndProc, trayMainProcCallback)
	if prev == 0 {
		cacheDebugLog("tray: SetWindowLongPtrW failed (%v); the close button will quit as before", errno)
		return
	}
	trayOriginalProc = prev
	traySubclassHWND = hwnd
}

func trayMainWindowProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	// A panic escaping a Win32 callback takes the process down, so this one
	// swallows its own failures and forwards instead.
	defer func() { _ = recover() }()

	if msg == trayWmSysCommand && wparam&0xfff0 == trayScClose {
		trayHideWindow()
		return 0
	}
	if trayOriginalProc != 0 {
		r, _, _ := procTrayCallWindowProc.Call(trayOriginalProc, hwnd, msg, wparam, lparam)
		return r
	}
	r, _, _ := procTrayDefWindowProc.Call(hwnd, msg, wparam, lparam)
	return r
}

func trayWindowProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	defer func() { _ = recover() }()

	switch msg {
	case trayWmCallback:
		switch lparam {
		case trayWmLButtonUp, trayWmLButtonDbl:
			trayShowWindow()
		case trayWmRButtonUp:
			trayShowMenu(hwnd)
		}
		return 0
	case trayWmCommand:
		switch wparam & 0xffff {
		case trayCmdShow:
			trayShowWindow()
		case trayCmdHide:
			trayHideWindow()
		case trayCmdQuit:
			if trayQuitFn != nil {
				trayQuitFn()
			}
		}
		return 0
	case trayWmDestroy:
		var nid notifyIconData
		nid.CbSize = uint32(unsafe.Sizeof(nid))
		nid.HWnd = trayMsgHWND
		nid.UID = 1
		_, _, _ = procTrayShellNotifyIcon.Call(trayNimDelete, uintptr(unsafe.Pointer(&nid)))
	}
	r, _, _ := procTrayDefWindowProc.Call(hwnd, msg, wparam, lparam)
	return r
}

func trayShowMenu(hwnd uintptr) {
	menu, _, _ := procTrayCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer func() { _, _, _ = procTrayDestroyMenu.Call(menu) }()

	visible, _, _ := procTrayIsWindowVisible.Call(trayMainHWND)
	showFlags := uintptr(trayMfString)
	hideFlags := uintptr(trayMfString)
	if visible != 0 {
		showFlags |= trayMfGrayed
	} else {
		hideFlags |= trayMfGrayed
	}
	_, _, _ = procTrayAppendMenu.Call(menu, showFlags, trayCmdShow, uintptr(unsafe.Pointer(&trayLabelShow[0])))
	_, _, _ = procTrayAppendMenu.Call(menu, hideFlags, trayCmdHide, uintptr(unsafe.Pointer(&trayLabelHide[0])))
	_, _, _ = procTrayAppendMenu.Call(menu, trayMfSeparator, 0, 0)
	_, _, _ = procTrayAppendMenu.Call(menu, trayMfString, trayCmdQuit, uintptr(unsafe.Pointer(&trayLabelQuit[0])))

	var pt POINT
	_, _, _ = procTrayGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	// The foreground call is required by the documented Shell_NotifyIcon
	// contract: without it the menu stays on screen when the user clicks
	// elsewhere. The trailing WM_NULL is the other half of that contract.
	_, _, _ = procSetFgWindow.Call(hwnd)
	_, _, _ = procTrayTrackPopupMenu.Call(
		menu,
		trayTpmRightButton|trayTpmBottomAlign,
		uintptr(pt.X), uintptr(pt.Y),
		0, hwnd, 0,
	)
	_, _, _ = procTrayPostMessage.Call(hwnd, trayWmNull, 0, 0)
}

func trayHideWindow() {
	if trayMainHWND == 0 {
		return
	}
	_, _, _ = procShowNormal.Call(trayMainHWND, traySwHide)
	cacheDebugLog("tray: window hidden; the app keeps running in the tray")
	if !trayToldUser {
		trayToldUser = true
		// Reuse the notification path the app already uses. Without a word the
		// window would simply seem to have vanished.
		go showNativeNotification("WhatsApp Desk is still running",
			"Right-click the tray icon to quit, or double-click it to reopen.",
			trayIconPath, trayExePath)
	}
}

func trayShowWindow() {
	if trayMainHWND == 0 {
		return
	}
	_, _, _ = procShowNormal.Call(trayMainHWND, traySwRestore)
	_, _, _ = procSetFgWindow.Call(trayMainHWND)
}
