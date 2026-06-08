package main

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	whKeyboardLL = 13

	wmKeydown    = 0x0100
	wmKeyup      = 0x0101
	wmSyskeydown = 0x0104
	wmSyskeyup   = 0x0105

	vkControl  = 0x11
	vkLControl = 0xA2
	vkRControl = 0xA3
	vkSpace    = 0x20
)

type kbdllhookstruct struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procSetWindowsHookEx = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx   = user32.NewProc("CallNextHookEx")
	procGetMessage       = user32.NewProc("GetMessageW")
	procGetAsyncKeyState = user32.NewProc("GetAsyncKeyState")
)

// runHook installs a low-level keyboard hook and pumps messages forever. On a ctrl+space
// press it sends true (duck) to cmd; on release it sends false (restore) — but only while
// settings.Enabled() is true, so the tray toggle can disarm it. The keypress is always
// passed through (CallNextHookEx) so Handy still receives ctrl+space. State is only touched
// on this single thread, so no synchronization is needed beyond Settings' own.
//
// It locks its own OS thread and runs its own message loop, so it must be its own goroutine
// — the main goroutine is reserved for the Walk UI's message loop.
// status receives the same hold transitions as cmd, so the UI can glow the tray icon while
// ctrl+space is actively held. Both sends are non-blocking.
func runHook(cmd chan<- bool, status chan<- bool, settings *Settings) error {
	runtime.LockOSThread()

	held := false // are we currently holding the duck/cable-cut active?

	callback := windows.NewCallback(func(nCode int32, wParam uintptr, lParam uintptr) uintptr {
		if nCode == 0 {
			ks := (*kbdllhookstruct)(unsafe.Pointer(lParam))
			switch wParam {
			case wmKeydown, wmSyskeydown:
				// Auto-repeat fires repeated keydowns while held; only act on the transition.
				if ks.VkCode == vkSpace && ctrlDown() && !held && settings.Enabled() {
					held = true
					send(cmd, true)
					send(status, true)
				}
			case wmKeyup, wmSyskeyup:
				if held && (ks.VkCode == vkSpace || ks.VkCode == vkControl || ks.VkCode == vkLControl || ks.VkCode == vkRControl) {
					held = false
					send(cmd, false)
					send(status, false)
				}
			}
		}
		ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
		return ret
	})

	hook, _, err := procSetWindowsHookEx.Call(uintptr(whKeyboardLL), callback, 0, 0)
	if hook == 0 {
		return fmt.Errorf("SetWindowsHookExW: %w", err)
	}

	// Message pump — required for the low-level hook callback to fire.
	var msg struct {
		Hwnd    uintptr
		Message uint32
		WParam  uintptr
		LParam  uintptr
		Time    uint32
		Pt      struct{ X, Y int32 }
	}
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 { // 0 = WM_QUIT, -1 = error
			return nil
		}
	}
}

// ctrlDown reports whether either Ctrl key is currently held.
func ctrlDown() bool {
	r, _, _ := procGetAsyncKeyState.Call(uintptr(vkControl))
	return r&0x8000 != 0
}

// send delivers a mute/unmute command without ever blocking the hook callback.
func send(cmd chan<- bool, v bool) {
	select {
	case cmd <- v:
	default:
	}
}
