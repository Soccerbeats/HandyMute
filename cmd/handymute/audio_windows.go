//go:build windows

package main

import (
	"runtime"
	"strings"

	ole "github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
	"golang.org/x/sys/windows"
)

// meetingApps are the process image names whose render volume the Meeting Volume slider
// controls. Matched case-insensitively against each session's owning process.
var meetingApps = map[string]bool{
	"teams.exe": true, "ms-teams.exe": true,
	"discord.exe":         true,
	"ts3client_win64.exe": true, "ts3client_win32.exe": true, "teamspeak.exe": true,
	"zoom.exe": true,
}

// applyMeetingVolume sets the per-application output (render) volume of any running meeting
// app to level (0..1) — the raw app volume, independent of the ctrl+space ducking, like the
// per-app slider in the Windows volume mixer. Windows persists it per app. Safe to call from
// the UI thread (COM is already up there); initCOM tolerates an existing apartment.
func applyMeetingVolume(level float32) {
	initCOM()

	var mmde *wca.IMMDeviceEnumerator
	if wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &mmde) != nil {
		return
	}
	defer mmde.Release()

	var col *wca.IMMDeviceCollection
	if mmde.EnumAudioEndpoints(wca.ERender, wca.DEVICE_STATE_ACTIVE, &col) != nil {
		return
	}
	defer col.Release()

	var n uint32
	col.GetCount(&n)
	set := 0
	for i := uint32(0); i < n; i++ {
		var dev *wca.IMMDevice
		if col.Item(i, &dev) != nil {
			continue
		}
		set += setMeetingSessions(dev, level)
		dev.Release()
	}
	logf("meeting volume -> %.0f%% on %d app session(s)", level*100, set)
}

// setMeetingSessions sets the volume of every session on dev owned by a meeting app.
func setMeetingSessions(dev *wca.IMMDevice, level float32) int {
	var asm2 *wca.IAudioSessionManager2
	if dev.Activate(wca.IID_IAudioSessionManager2, wca.CLSCTX_ALL, nil, &asm2) != nil {
		return 0
	}
	defer asm2.Release()

	var enum *wca.IAudioSessionEnumerator
	if asm2.GetSessionEnumerator(&enum) != nil {
		return 0
	}
	defer enum.Release()

	var count int
	enum.GetCount(&count)
	set := 0
	for i := 0; i < count; i++ {
		var asc *wca.IAudioSessionControl
		if enum.GetSession(i, &asc) != nil {
			continue
		}
		if isMeetingSession(asc) {
			var sav *wca.ISimpleAudioVolume
			if asc.PutQueryInterface(wca.IID_ISimpleAudioVolume, &sav) == nil {
				if sav.SetMasterVolume(level, nil) == nil {
					set++
				}
				sav.Release()
			}
		}
		asc.Release()
	}
	return set
}

// isMeetingSession reports whether the session is owned by a known meeting app.
func isMeetingSession(asc *wca.IAudioSessionControl) bool {
	var asc2 *wca.IAudioSessionControl2
	if asc.PutQueryInterface(wca.IID_IAudioSessionControl2, &asc2) != nil {
		return false
	}
	defer asc2.Release()

	var pid uint32
	if asc2.GetProcessId(&pid) != nil || pid == 0 {
		return false
	}
	return meetingApps[strings.ToLower(processName(pid))]
}

// processName resolves a PID to its executable's base name (e.g. "Discord.exe").
func processName(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)

	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	full := windows.UTF16ToString(buf[:size])
	if idx := strings.LastIndexAny(full, `\/`); idx >= 0 {
		return full[idx+1:]
	}
	return full
}

// muteWorker owns all COM/WASAPI work on a single dedicated thread. It receives desired
// hold states from the keyboard hook and applies two effects while ctrl+space is held:
//
//  1. Lowers the VB-Cable render endpoint to settings.TeamsLevel, so the call apps (which
//     read CABLE Output) hear you at that level — 0% is silent, higher lets teammates still
//     hear you quietly. Handy reads the physical mic directly and is never touched.
//  2. Lowers the master volume of every other output device to settings.SpeakerDuck, so all
//     audio in your ears (the call, YouTube, music) is quieter while you dictate.
//
// Both effects are reversed on release, restoring each device's exact prior level. State
// lives only on this thread, so no locking is needed beyond Settings' own.
func muteWorker(cmd <-chan bool, settings *Settings) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		logf("CoInitializeEx failed: %v", err)
		return
	}
	defer ole.CoUninitialize()

	saved := map[string]float32{} // device id -> volume scalar captured before ducking

	for desired := range cmd {
		if err := apply(desired, settings.Snapshot(), saved); err != nil {
			logf("apply(%v) error: %v", desired, err)
		}
	}
}

// apply ducks (hold) or restores (release) every active output device. The VB-Cable feed is
// driven to snap.TeamsLevel; every other output to snap.SpeakerDuck.
func apply(hold bool, snap Snapshot, saved map[string]float32) error {
	var mmde *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &mmde); err != nil {
		return err
	}
	defer mmde.Release()

	var col *wca.IMMDeviceCollection
	if err := mmde.EnumAudioEndpoints(wca.ERender, wca.DEVICE_STATE_ACTIVE, &col); err != nil {
		return err
	}
	defer col.Release()

	var count uint32
	if err := col.GetCount(&count); err != nil {
		return err
	}

	cable, ducked := 0, 0
	for i := uint32(0); i < count; i++ {
		var dev *wca.IMMDevice
		if col.Item(i, &dev) != nil {
			continue
		}
		name := friendlyName(dev)
		if strings.Contains(strings.ToLower(name), snap.CableMatch) {
			if applyCable(dev, name, hold, snap.TeamsLevel) {
				cable++
			}
		} else if applyDuck(dev, name, deviceID(dev), hold, snap.SpeakerDuck, saved) {
			ducked++
		}
		dev.Release()
	}

	verb := "released"
	if hold {
		verb = "held"
	}
	logf("%s: cable feed on %d device(s), ducked %d output device(s)", verb, cable, ducked)
	return nil
}

// applyCable controls how loudly teammates hear you while you dictate. VB-Cable ignores its
// endpoint (device) volume for the audio it forwards, but it cannot ignore PER-SESSION
// volume: the Windows audio engine attenuates each session in software before the cable
// driver ever sees it. The "Listen to this device" feed is a session on CABLE Input, so we
// scale that session to the desired level — giving real partial volume, not just on/off.
//
// At level 0 we additionally mute the endpoint as a belt-and-suspenders guarantee of true
// silence (the proven-reliable path). On release the cable returns to full volume, unmuted.
func applyCable(dev *wca.IMMDevice, name string, hold bool, level float32) bool {
	aev := endpointVolume(dev)
	if aev == nil {
		return false
	}
	defer aev.Release()

	if !hold {
		aev.SetMute(false, nil)
		n := setCableSessions(dev, 1.0)
		logf("  cable %q -> on (unmuted, 100%% across %d session(s))", name, n)
		return true
	}

	n := setCableSessions(dev, level)
	if level <= 0 {
		aev.SetMute(true, nil) // guarantee silence regardless of session behavior
		logf("  cable %q -> muted (teammates hear silence)", name)
		return true
	}

	aev.SetMute(false, nil)
	logf("  cable %q -> %.0f%% across %d session(s) (teammates hear you quietly)", name, level*100, n)
	return true
}

// setCableSessions sets the per-session volume of every audio session on the cable's render
// endpoint to level (0..1), unmuting each. The cable is dedicated to the mic feed, so every
// session on it is part of that feed. Returns the number of sessions adjusted. This is what
// produces genuine partial attenuation, since session volume is applied engine-side.
func setCableSessions(dev *wca.IMMDevice, level float32) int {
	var asm2 *wca.IAudioSessionManager2
	if err := dev.Activate(wca.IID_IAudioSessionManager2, wca.CLSCTX_ALL, nil, &asm2); err != nil {
		return 0
	}
	defer asm2.Release()

	var enum *wca.IAudioSessionEnumerator
	if err := asm2.GetSessionEnumerator(&enum); err != nil {
		return 0
	}
	defer enum.Release()

	var n int
	if err := enum.GetCount(&n); err != nil {
		return 0
	}

	set := 0
	for i := 0; i < n; i++ {
		var asc *wca.IAudioSessionControl
		if enum.GetSession(i, &asc) != nil {
			continue
		}
		var sav *wca.ISimpleAudioVolume
		if asc.PutQueryInterface(wca.IID_ISimpleAudioVolume, &sav) == nil {
			sav.SetMute(false, nil)
			if sav.SetMasterVolume(level, nil) == nil {
				set++
			}
			sav.Release()
		}
		asc.Release()
	}
	return set
}

// applyDuck drives one output device to target (0..1) while held, and restores its captured
// level on release. Returns true if it acted on the device.
func applyDuck(dev *wca.IMMDevice, name, id string, hold bool, target float32, saved map[string]float32) bool {
	aev := endpointVolume(dev)
	if aev == nil {
		return false
	}
	defer aev.Release()

	if hold {
		var cur float32
		if err := aev.GetMasterVolumeLevelScalar(&cur); err != nil {
			return false
		}
		saved[id] = cur
		if err := aev.SetMasterVolumeLevelScalar(target, nil); err != nil {
			logf("  duck %q failed: %v", name, err)
			return false
		}
		logf("  %q -> %.0f%% (was %.0f%%)", name, target*100, cur*100)
		return true
	}

	// Release: restore the level captured on the way in. If we never saw this device
	// (e.g. it appeared mid-hold), leave it alone.
	prev, ok := saved[id]
	if !ok {
		return false
	}
	delete(saved, id)
	if err := aev.SetMasterVolumeLevelScalar(prev, nil); err != nil {
		logf("  restore %q failed: %v", name, err)
		return false
	}
	logf("  restore %q -> %.0f%%", name, prev*100)
	return true
}

// endpointVolume activates the IAudioEndpointVolume control for a device, or nil on failure.
// The caller must Release the returned interface.
func endpointVolume(dev *wca.IMMDevice) *wca.IAudioEndpointVolume {
	var aev *wca.IAudioEndpointVolume
	if err := dev.Activate(wca.IID_IAudioEndpointVolume, wca.CLSCTX_ALL, nil, &aev); err != nil {
		return nil
	}
	return aev
}

// friendlyName returns a device's human-readable name (e.g. "CABLE Input (VB-Audio Virtual
// Cable)"), or "" if it can't be read.
func friendlyName(dev *wca.IMMDevice) string {
	var ps *wca.IPropertyStore
	if err := dev.OpenPropertyStore(wca.STGM_READ, &ps); err != nil {
		return ""
	}
	defer ps.Release()

	var pv wca.PROPVARIANT
	if err := ps.GetValue(&wca.PKEY_Device_FriendlyName, &pv); err != nil {
		return ""
	}
	return pv.String()
}

// deviceID returns a device's stable endpoint ID string, used as the key for remembering
// pre-duck volume across the hold.
func deviceID(dev *wca.IMMDevice) string {
	var id string
	if err := dev.GetId(&id); err != nil {
		return ""
	}
	return id
}
