//go:build linux

package main

/*
#cgo pkg-config: x11 xtst
#include <X11/Xlib.h>
#include <X11/extensions/record.h>
#include <stdlib.h>

extern void goRecordKey(int type, int keycode, int state);

// recordCallback receives raw protocol bytes for each KeyPress/KeyRelease.
static void recordCallback(XPointer closure, XRecordInterceptData *d) {
    if (d->category == XRecordFromServer && d->data_len * 4 >= 30) {
        unsigned char *p = (unsigned char*)d->data;
        int type    = p[0] & 0x7f;          // 2 = KeyPress, 3 = KeyRelease
        int keycode = p[1];
        int state   = p[28] | (p[29] << 8); // modifier mask at event time (ControlMask = 0x4)
        goRecordKey(type, keycode, state);
    }
    XRecordFreeData(d);
}

// runRecord opens two X connections (control + data) and blocks delivering key events.
static int runRecord() {
    Display *ctrlDpy = XOpenDisplay(NULL);
    Display *dataDpy = XOpenDisplay(NULL);
    if (!ctrlDpy || !dataDpy) return 1;

    XRecordRange *rr = XRecordAllocRange();
    if (!rr) return 2;
    rr->device_events.first = KeyPress;
    rr->device_events.last  = KeyRelease;

    XRecordClientSpec clients = XRecordAllClients;
    XRecordContext ctx = XRecordCreateContext(ctrlDpy, 0, &clients, 1, &rr, 1);
    XFree(rr);
    XSync(ctrlDpy, False);

    if (!XRecordEnableContext(dataDpy, ctx, recordCallback, NULL)) return 3;
    return 0;
}

// spaceKeycode resolves the keycode for the space keysym.
static int spaceKeycode() {
    Display *d = XOpenDisplay(NULL);
    if (!d) return 0;
    int kc = XKeysymToKeycode(d, XStringToKeysym("space"));
    XCloseDisplay(d);
    return kc;
}
*/
import "C"

import (
	"fmt"
	"runtime"
)

const controlMask = 0x04 // X11 ControlMask

// Hook singleton state (the C callback has no user-data path to Go, so we use package vars).
var (
	hookCmd      chan<- bool
	hookStatus   chan<- bool
	hookSettings *Settings
	hookSpaceKC  int
	hookHeld     bool
)

//export goRecordKey
func goRecordKey(typ, keycode, state C.int) {
	if int(keycode) != hookSpaceKC {
		return
	}
	isPress := int(typ) == 2 // KeyPress
	ctrl := int(state)&controlMask != 0

	if isPress && ctrl && !hookHeld && hookSettings.Enabled() {
		hookHeld = true
		send(hookCmd, true)
		send(hookStatus, true)
	} else if !isPress && hookHeld {
		hookHeld = false
		send(hookCmd, false)
		send(hookStatus, false)
	}
}

// runHook passively watches Ctrl+Space via the X RECORD extension. Ctrl+Space is never
// grabbed, so Handy still receives it. Ctrl is read from each space event's modifier state
// field (Handy grabs Ctrl first, so tracking the Ctrl keycode directly is unreliable).
func runHook(cmd chan<- bool, status chan<- bool, settings *Settings) error {
	runtime.LockOSThread()
	hookCmd, hookStatus, hookSettings = cmd, status, settings

	hookSpaceKC = int(C.spaceKeycode())
	if hookSpaceKC == 0 {
		return fmt.Errorf("could not resolve space keycode (no X display?)")
	}
	logf("hook: watching ctrl+space via XRecord (space keycode=%d)", hookSpaceKC)

	if rc := int(C.runRecord()); rc != 0 {
		return fmt.Errorf("XRecord setup failed (code=%d)", rc)
	}
	return fmt.Errorf("XRecord context ended")
}
