//go:build linux

package main

/*
#cgo pkg-config: gtk+-3.0 webkit2gtk-4.1
#include "gtk_linux.h"
*/
import "C"

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"unsafe"
)

var (
	uiSettings    *Settings
	uiCmd         chan<- bool
	iconDir       string
	linuxOverlay  *statusOverlay
)

// ---- UI glue ----

//export goWebMessage
func goWebMessage(msg *C.char) {
	raw := C.GoString(msg)
	var m struct {
		Action string `json:"action"`
	}
	if json.Unmarshal([]byte(raw), &m) == nil {
		switch m.Action {
		case "hide":
			C.ui_hide()
			return
		case "quit":
			C.gtk_main_quit()
			return
		}
	}
	handleControlMessage(raw, uiSettings, uiCmd, uiEval, setLinuxGlow)
}

//export goMenuOpen
func goMenuOpen() { C.ui_show() }

//export goMenuQuit
func goMenuQuit() { C.gtk_main_quit() }

func uiEval(js string) {
	cjs := C.CString(js)
	defer C.free(unsafe.Pointer(cjs))
	C.ui_eval(cjs)
}

func trayIconPath(active bool) string {
	name := "handymute.png"
	if active {
		name = "handymute-active.png"
	}
	return filepath.Join(iconDir, name)
}

func setTrayIcon(active bool) {
	cpath := C.CString(trayIconPath(active))
	defer C.free(unsafe.Pointer(cpath))
	C.ui_tray_set_icon(cpath)
}

func setLinuxGlow(on bool) { setTrayIcon(on) }

func runUI(settings *Settings, cmd chan<- bool, status <-chan bool) error {
	runtime.LockOSThread()
	uiSettings, uiCmd = settings, cmd

	var err error
	iconDir, err = writeTrayIcons()
	if err != nil {
		logf("writeTrayIcons: %v", err)
	}

	chtml := C.CString(controlCenterHTML)
	defer C.free(unsafe.Pointer(chtml))
	C.ui_init(chtml)

	cicon := C.CString(trayIconPath(false))
	C.ui_tray_init(cicon)
	C.free(unsafe.Pointer(cicon))

	if ov, err := newStatusOverlay(settings); err != nil {
		logf("overlay init failed (non-fatal): %v", err)
	} else {
		linuxOverlay = ov
	}

	go func() {
		for active := range status {
			setLinuxGlow(active)
			if active {
				uiEval("setActive(true)")
				if linuxOverlay != nil {
					linuxOverlay.Show()
				}
			} else {
				uiEval("setActive(false)")
				if linuxOverlay != nil {
					linuxOverlay.Hide()
				}
			}
		}
	}()

	C.ui_run()
	return nil
}
