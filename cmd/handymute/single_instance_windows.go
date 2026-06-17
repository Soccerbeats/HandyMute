//go:build windows

package main

import (
	"errors"

	"golang.org/x/sys/windows"
)

// Per-session names: one HandyMute per logged-in user session (a tray app), not per machine.
const (
	singletonMutexName = `Local\HandyMute_SingleInstance`
	activateEventName  = `Local\HandyMute_Activate`
)

// activateEvent is the auto-reset event the first instance waits on; a second instance signals
// it to ask the first to surface its panel.
var activateEvent windows.Handle

// claimSingleInstance returns true if this is the first HandyMute instance. If another is
// already running, it signals that instance to open its Control Center and returns false (the
// caller should exit). The mutex handle is intentionally left open for the process lifetime.
func claimSingleInstance() bool {
	name, err := windows.UTF16PtrFromString(singletonMutexName)
	if err != nil {
		return true // can't build name; don't block startup
	}
	if _, err := windows.CreateMutex(nil, false, name); errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		// Another instance owns the mutex — tell it to surface, then bow out.
		if evName, e := windows.UTF16PtrFromString(activateEventName); e == nil {
			if h, e := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, evName); e == nil {
				windows.SetEvent(h)
				windows.CloseHandle(h)
			}
		}
		return false
	}

	// We're first: create the activation event others will signal (auto-reset, nonsignaled).
	if evName, e := windows.UTF16PtrFromString(activateEventName); e == nil {
		activateEvent, _ = windows.CreateEvent(nil, 0, 0, evName)
	}
	return true
}

// waitForActivate blocks forever; each time another instance signals the activation event it
// invokes show(). Intended to run in its own goroutine.
func waitForActivate(show func()) {
	if activateEvent == 0 {
		return
	}
	for {
		s, err := windows.WaitForSingleObject(activateEvent, windows.INFINITE)
		if err != nil || s != windows.WAIT_OBJECT_0 {
			return
		}
		show()
	}
}
