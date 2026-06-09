//go:build windows

package main

import "github.com/lxn/walk"

// loadIcons builds the two tray icons entirely in code: a neutral white microphone, and a
// green microphone wrapped in a green glow shown while ctrl+space is held.
func loadIcons() (normal, glow *walk.Icon) {
	if ic, err := walk.NewIconFromImageForDPI(micCanvas(micIdle), 96); err == nil {
		normal = ic
	}
	if ic, err := walk.NewIconFromImageForDPI(withHalo(micCanvas(micGreen), micGreen), 96); err == nil {
		glow = ic
	}
	return normal, glow
}
