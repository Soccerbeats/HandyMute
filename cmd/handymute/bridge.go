//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	ole "github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
	"golang.org/x/sys/windows/registry"
)

// initCOM initializes COM on the current (locked) thread, tolerating the case where the
// thread is already in an apartment (CoInitializeEx then returns S_FALSE/RPC_E_CHANGED_MODE).
// WASAPI enumeration works in either apartment. These commands exit the process when done,
// so we deliberately don't CoUninitialize (which would unbalance a pre-existing apartment).
func initCOM() {
	runtime.LockOSThread()
	_ = ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
}

// The "Listen to this device" feature is undocumented but stores its state in the capture
// device's registry property store. These are the two property keys that matter:
//
//	{24DBB0FC-...},1  REG_BINARY  serialized VT_BOOL — the "Listen to this device" checkbox
//	{24DBB0FC-...},0  REG_SZ      the render endpoint ID to play the mic through
//
// The byte blob below is exactly what the Windows UI writes for an enabled checkbox.
const (
	listenPropSet     = "{24DBB0FC-9311-4B3D-9CF0-18FF155639D4}"
	listenEnabledName = listenPropSet + ",1"
	listenTargetName  = listenPropSet + ",0"
	mmDevCaptureKey   = `SOFTWARE\Microsoft\Windows\CurrentVersion\MMDevices\Audio\Capture\`
)

var listenEnabledBlob = []byte{0x0B, 0, 0, 0, 0x01, 0, 0, 0, 0xFF, 0xFF, 0, 0}

// setupBridge routes the default microphone into the VB-Cable feed by enabling Windows
// "Listen to this device" → CABLE Input, so the call apps (reading CABLE Output) hear the
// mic. This is what the installer runs so the user never has to open Sound settings. It
// requires administrator rights (HKLM write) and restarts the audio service to apply.
func setupBridge() error {
	initCOM()

	micID, micName, err := defaultCaptureEndpoint()
	if err != nil {
		return fmt.Errorf("finding default microphone: %w", err)
	}
	cableID := renderEndpointID("cable input")
	if cableID == "" {
		return fmt.Errorf("CABLE Input not found — is VB-Cable installed?")
	}

	guid := endpointGUID(micID)
	if guid == "" {
		return fmt.Errorf("could not parse mic endpoint id %q", micID)
	}

	k, err := registry.OpenKey(registry.LOCAL_MACHINE, mmDevCaptureKey+guid+`\Properties`, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("opening mic property store (need admin?): %w", err)
	}
	defer k.Close()

	if err := k.SetBinaryValue(listenEnabledName, listenEnabledBlob); err != nil {
		return fmt.Errorf("enabling listen: %w", err)
	}
	if err := k.SetStringValue(listenTargetName, cableID); err != nil {
		return fmt.Errorf("setting listen target: %w", err)
	}

	logf("bridge: %q (mic %s) -> Listen to CABLE Input (%s)", micName, guid, cableID)
	restartAudioService()
	return nil
}

// removeBridge disables the pass-through by deleting the Listen properties from the default
// microphone (reverting to the Windows default of "off"). Used by the uninstaller.
func removeBridge() error {
	initCOM()

	micID, _, err := defaultCaptureEndpoint()
	if err != nil {
		return err
	}
	guid := endpointGUID(micID)
	if guid == "" {
		return fmt.Errorf("could not parse mic endpoint id %q", micID)
	}

	k, err := registry.OpenKey(registry.LOCAL_MACHINE, mmDevCaptureKey+guid+`\Properties`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	k.DeleteValue(listenEnabledName) // best-effort; absent == off
	k.DeleteValue(listenTargetName)
	logf("bridge: removed Listen pass-through from mic %s", guid)
	restartAudioService()
	return nil
}

// defaultCaptureEndpoint returns the endpoint ID and friendly name of the default recording
// device (the mic the dictation app uses).
func defaultCaptureEndpoint() (id, name string, err error) {
	var mmde *wca.IMMDeviceEnumerator
	if err = wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &mmde); err != nil {
		return
	}
	defer mmde.Release()

	var dev *wca.IMMDevice
	if err = mmde.GetDefaultAudioEndpoint(wca.ECapture, wca.EConsole, &dev); err != nil {
		return
	}
	defer dev.Release()

	if err = dev.GetId(&id); err != nil {
		return
	}
	name = friendlyName(dev)
	return id, name, nil
}

// renderEndpointID returns the full endpoint ID string of the active render device whose
// friendly name contains match (case-insensitive), or "" if none.
func renderEndpointID(match string) string {
	var mmde *wca.IMMDeviceEnumerator
	if wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &mmde) != nil {
		return ""
	}
	defer mmde.Release()

	var col *wca.IMMDeviceCollection
	if mmde.EnumAudioEndpoints(wca.ERender, wca.DEVICE_STATE_ACTIVE, &col) != nil {
		return ""
	}
	defer col.Release()

	var n uint32
	col.GetCount(&n)
	for i := uint32(0); i < n; i++ {
		var dev *wca.IMMDevice
		if col.Item(i, &dev) != nil {
			continue
		}
		name := friendlyName(dev)
		var id string
		dev.GetId(&id)
		dev.Release()
		if strings.Contains(strings.ToLower(name), match) {
			return id
		}
	}
	return ""
}

// endpointGUID extracts the trailing {guid} from a WASAPI endpoint ID such as
// "{0.0.1.00000000}.{955de4e6-...}", which is the subkey name under the MMDevices registry.
func endpointGUID(endpointID string) string {
	if i := strings.LastIndex(endpointID, ".{"); i >= 0 {
		return endpointID[i+1:]
	}
	return ""
}

// restartAudioService cycles the audio engine so a freshly written Listen setting takes
// effect immediately. Restarting AudioEndpointBuilder also restarts its dependent Audiosrv.
func restartAudioService() {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		"Restart-Service -Force -Name AudioEndpointBuilder")
	if out, err := cmd.CombinedOutput(); err != nil {
		logf("bridge: audio service restart failed (a sign-out/in will also apply it): %v: %s", err, strings.TrimSpace(string(out)))
		return
	}
	logf("bridge: audio service restarted")
}
