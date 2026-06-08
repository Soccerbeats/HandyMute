// handymute — single-executable replacement for the AutoHotkey + svcl daemon stack.
//
// While ctrl+space is held (Handy's push-to-talk dictation key) it does two things so you
// can dictate to Handy without disrupting a call:
//
//  1. Mutes the VB-Cable render endpoint, so call apps (Teams/Zoom/Discord, which read
//     CABLE Output) hear silence. Handy reads the physical mic directly and is unaffected.
//  2. Ducks the master volume of every output device, so all audio in your ears (the call,
//     YouTube, music) drops while you dictate, then restores on release.
//
// The ctrl+space keypress is always passed through to Handy. The keyboard hook and all
// WASAPI work are done in-process — no extra programs. The one prerequisite is VB-Cable
// installed and the call apps pointed at CABLE Output (a virtual sound card can't be
// created from user space).
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	install := flag.Bool("install", false, "register handymute to auto-start at login, then exit")
	uninstall := flag.Bool("uninstall", false, "remove the auto-start registration, then exit")
	setupBridgeFlag := flag.Bool("setup-bridge", false, "route the default mic into CABLE Input via Windows Listen, then exit (needs admin)")
	removeBridgeFlag := flag.Bool("remove-bridge", false, "remove the mic→CABLE Input Listen pass-through, then exit (needs admin)")
	flag.Parse()

	if *setupBridgeFlag {
		if err := setupBridge(); err != nil {
			fmt.Fprintln(os.Stderr, "setup-bridge failed:", err)
			os.Exit(1)
		}
		fmt.Println("Microphone pass-through to CABLE Input enabled.")
		return
	}
	if *removeBridgeFlag {
		if err := removeBridge(); err != nil {
			fmt.Fprintln(os.Stderr, "remove-bridge failed:", err)
			os.Exit(1)
		}
		fmt.Println("Microphone pass-through removed.")
		return
	}

	if *install {
		if err := installStartup(); err != nil {
			fmt.Fprintln(os.Stderr, "install failed:", err)
			os.Exit(1)
		}
		fmt.Println("handymute registered to start at login.")
		return
	}
	if *uninstall {
		if err := uninstallStartup(); err != nil {
			fmt.Fprintln(os.Stderr, "uninstall failed:", err)
			os.Exit(1)
		}
		fmt.Println("handymute auto-start removed.")
		return
	}

	settings := loadSettings()
	snap := settings.Snapshot()
	logf("handymute starting. enabled=%t  teammates=%.0f%%  my-volume=%.0f%%  cable=%q",
		settings.Enabled(), snap.TeamsLevel*100, snap.SpeakerDuck*100, snap.CableMatch)

	// Worker owns all COM/WASAPI work on its own thread; the keyboard hook only signals it.
	cmd := make(chan bool, 64)
	status := make(chan bool, 64) // hold transitions, for the UI's glow indicator
	go muteWorker(cmd, settings)

	// The hook locks its own OS thread and runs its own message loop, so it runs on its own
	// goroutine — the main goroutine is reserved for the Walk UI message loop below.
	go func() {
		if err := runHook(cmd, status, settings); err != nil {
			logf("hook failed: %v", err)
			fmt.Fprintln(os.Stderr, "hook failed:", err)
			os.Exit(1)
		}
	}()

	// runUI builds the tray + settings window and pumps the UI message loop until Quit.
	if err := runUI(settings, cmd, status); err != nil {
		logf("ui failed: %v", err)
		fmt.Fprintln(os.Stderr, "ui failed:", err)
		os.Exit(1)
	}
}
