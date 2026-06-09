# Porting HandyMute to another OS

HandyMute is split into a **platform-agnostic core** and **per-OS backend files** selected
by Go build tags. To add a platform (e.g. Linux), implement the functions below in new files
tagged for that OS — you do **not** modify the core or the existing Windows files.

## Shared core (no OS dependencies — don't touch for a port)

| File | Contents |
|---|---|
| `main.go` | entry point, flag parsing, wiring (calls the backend functions below) |
| `settings.go` | `Settings` type (Enabled, SpeakerDuck, TeamsLevel, Theme), persistence to `handymute.conf` |
| `config.go` | `exeDir()`, `logf()` |
| `controlcenter.html` | the Control Center UI (embedded; reusable by any webview-based UI) |

## The backend contract

The Windows backend lives in files tagged `//go:build windows` (`hook.go`, `audio.go`,
`ui.go`, `icons.go`, `bridge.go`, `install.go`). A new backend provides files tagged for its
OS (e.g. `//go:build linux`) defining these **exact signatures**:

```go
// Global hotkey listener. Send true to cmd on Ctrl+Space press, false on release — but only
// while settings.Enabled(). Also mirror those transitions to status (for the UI glow).
// Must pass the keypress through so the dictation app still receives it. Blocks forever.
func runHook(cmd chan<- bool, status chan<- bool, settings *Settings) error

// Owns audio. Receives hold(true)/release(false) and applies the effects below, then reverses
// them on release. Reads live values via settings.Snapshot().
func muteWorker(cmd <-chan bool, settings *Settings)

// Builds the tray icon + Control Center UI and runs the UI event loop until quit. Blocks.
func runUI(settings *Settings, cmd chan<- bool, status <-chan bool) error

// Auto-start at login.
func installStartup() error
func uninstallStartup() error
func startupEnabled() bool

// Configure / remove the mic→call-app routing so the app can attenuate it (see note for Linux).
func setupBridge() error
func removeBridge() error
```

### What the audio backend must do while held

- **Teammates level:** drive what the call app hears to `Snapshot().TeamsLevel` (0.0 = silent,
  1.0 = full). The dictation app (Handy) must keep full mic.
- **Duck output:** drop every output device to `Snapshot().SpeakerDuck` so all the user's audio
  (call, music, video) dims. Restore exact prior levels on release.

## Linux notes (PipeWire)

The Windows design needs a virtual cable (VB-CABLE) because Windows' per-app *capture* mute
isn't isolated. **Linux doesn't have that problem** — PipeWire/PulseAudio give each recording
app its own software-applied volume:

- **Audio:** attenuate the call app's capture stream directly (`pactl`/`wpctl` — set the
  source-output volume of the Teams/Zoom stream; leave the dictation app's stream alone). Duck
  sinks / sink-inputs for "my volume." **No virtual cable needed**, so `setupBridge`/
  `removeBridge` can be no-ops (or create a null sink if you prefer that approach).
- **Hotkey:** reading `evdev` (`/dev/input/event*`) works on both X11 and Wayland (Wayland has
  no standard global-hotkey API); requires the user in the `input` group.
- **UI:** WebView2 is Windows-only. Host the shared `controlcenter.html` in **WebKitGTK** (cgo)
  for the same look, or use another toolkit. The JSON message bridge in the HTML
  (`window.external.invoke` ↔ Go `MessageCallback`) is the integration point.
- **Autostart:** a `.desktop` file in `~/.config/autostart/`.

## Verifying the split

```bash
GOOS=windows go build ./cmd/handymute   # builds (Windows backend present)
GOOS=linux   go build ./cmd/handymute   # fails until the linux backend is added — by design
```
