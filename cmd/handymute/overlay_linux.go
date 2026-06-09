//go:build linux

package main

/*
#cgo pkg-config: gtk+-3.0
#include "gtk_linux.h"
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// statusOverlay is the Linux implementation of the status bubble. It uses a GTK POPUP
// window with a Pango-markup label — no WebKit, so no extra renderer overhead.
type statusOverlay struct {
	settings *Settings
}

// newStatusOverlay creates and hides the overlay window. Must be called after gtk_init
// (i.e. after ui_init), on the UI goroutine.
func newStatusOverlay(settings *Settings) (*statusOverlay, error) {
	C.overlay_init()
	return &statusOverlay{settings: settings}, nil
}

// Show positions and displays the overlay with current volume levels.
// Safe to call from any goroutine — the C layer dispatches via g_idle_add.
func (o *statusOverlay) Show() {
	snap := o.settings.Snapshot()

	outLabel := fmt.Sprintf("out %d%%", pct(snap.TeamsLevel))
	if snap.TeamsLevel <= 0 {
		outLabel = "muted"
	}

	markup := fmt.Sprintf(
		`<span foreground="#34c759" font_weight="bold" font_size="11264">● Active</span>`+
			`<span foreground="#ababbe" font_size="11264">  ·  %s · in %d%%</span>`,
		outLabel, pct(snap.SpeakerDuck))

	cs := C.CString(markup)
	defer C.free(unsafe.Pointer(cs))
	C.overlay_show(cs)
}

// Hide removes the overlay from screen.
// Safe to call from any goroutine.
func (o *statusOverlay) Hide() {
	C.overlay_hide()
}
