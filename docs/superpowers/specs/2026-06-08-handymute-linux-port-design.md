# HandyMute Linux Port — Design

**Date:** 2026-06-08
**Status:** Approved, pre-implementation
**Target (phase 1):** Pop!_OS / X11, GNOME. Wayland deferred to a later phase.

## Goal

Add Linux support to HandyMute alongside the existing Windows code — same repo, same
`package main` — so it builds to a native binary on both platforms and can be merged
upstream (github.com/Soccerbeats/HandyMute) as a clean contribution. No separate fork.

Two hard requirements from the maintainer (Dan):

1. **Works on most/all Linux installations**, not just this machine.
2. **No special user setup** wherever avoidable — the user must not have to manually point
   call apps at a virtual mic or otherwise fiddle with audio settings.

## Behavior (unchanged from Windows)

While Ctrl+Space (Handy's push-to-talk dictation key) is held:

- Teammates on the call hear silence, or a quiet level the user chooses (`teams_level`,
  default 0% = silent).
- All of the user's own output audio dims to `speaker_duck` (default 20%) so they can hear
  themselves dictate.
- Everything restores instantly on release.
- Ctrl+Space is always passed through to Handy; Handy reads the real mic and is never
  affected.

The existing settings model (`enabled`, `speaker_duck`, `teams_level`, `theme`,
start-at-login) and the `controlcenter.html` Control Center are reused as-is.

## Architecture

### Code structure (upstream-friendly)

Use Go's filename build constraints rather than scattered build tags. Rename the existing
Windows files to a `*_windows.go` suffix and add Linux counterparts with `*_linux.go`.

| Concern             | Windows (existing → renamed)        | Linux (new)        |
|---------------------|-------------------------------------|--------------------|
| Entry / flag parse  | `main.go` (shared, slimmed)         | shared             |
| Settings / config   | `settings.go`, `config.go` (shared) | shared             |
| Hotkey watcher      | `hook.go` → `hook_windows.go`       | `hook_linux.go`    |
| Audio worker        | `audio.go` → `audio_windows.go`     | `audio_linux.go`   |
| UI + tray           | `ui.go` → `ui_windows.go`           | `ui_linux.go`      |
| Tray icons          | `icons.go` → `icons_windows.go`     | `icons_linux.go`   |
| Start-at-login      | `install.go` → `install_windows.go` | `install_linux.go` |
| Virtual-cable bridge| `bridge.go` → `bridge_windows.go`   | *(none)*           |
| Control Center HTML | `controlcenter.html` (shared)       | shared             |

Each platform file satisfies the same small internal contract that `main.go` calls:

- a hotkey watcher that emits hold-on / hold-off events,
- an audio worker that applies / reverses the duck on those events,
- `runUI(settings, cmd, status)`,
- `installStartup()` / `uninstallStartup()` / `startupEnabled()`.

The Windows-only `-setup-bridge` / `-remove-bridge` flags become no-ops on Linux (the
virtual cable does not exist there).

**Rejected:** a separate Linux fork — fails the upstream-contribution goal.

### Audio (`audio_linux.go`) — zero-config, PulseAudio protocol

The worker speaks the **PulseAudio protocol** (via `pactl`/libpulse), which is served by
both PipeWire (Pop!_OS, modern Ubuntu/Fedora) and classic PulseAudio systems — one code
path, no PipeWire-specific dependency.

On **hold**:

- **Teammates:** enumerate capture streams (source-outputs). For every stream whose owning
  application is **not Handy** (match `application.process.binary == "handy"`, or
  `application.name` / `node.name` containing "handy", case-insensitive), save its current
  volume and set it to `teams_level`. Restore the exact saved value on release.
- **Your ears:** save the default sink's current volume, set it to `speaker_duck`, restore
  on release.

Streams are re-enumerated on every press, so newly appeared or drifted streams are always
caught. Because the worker only ever acts on non-Handy streams, Handy's capture (which
opens at the same instant the hold begins) is never touched — no race, and immune to the
Chrome WebRTC device-drift problem that broke device-routing approaches previously.

**Rejected:** virtual null-sink + loopback + virtual-source (the previously working
hand-built approach). It requires the user to point each call app at "Handy-Virtual-Mic" —
exactly the manual setup requirement 2 forbids.

**Known limitation (v1):** any other capturer (e.g. OBS, a second recorder) is also
silenced during a hold. Acceptable for v1; a per-app exclusion list can be added later.

### Hotkey (`hook_linux.go`) — X11, zero-setup

Passively monitor key events via the **X RECORD extension** (in-process, via cgo against
`libX11` / `libXtst`). Detect `space` key press and release, and read the **Ctrl modifier
state from the event's state field** rather than tracking the Ctrl keycode — necessary
because Handy grabs Ctrl via `XGrabKey` before a passive observer would see it. Ctrl+Space
is never intercepted, so Handy still receives it.

Emits hold-on when (space pressed AND Ctrl active) and `enabled` is true; hold-off on space
release. When `enabled` is false, events pass straight through (no ducking).

**Rejected:** `XGrabKey` (collides with Handy's own grab); evdev `/dev/input` (requires the
user's account in the `input` group — a setup step; reserved as the Wayland-phase tool).

### UI (`ui_linux.go`, `icons_linux.go`) — reuse the Control Center

- Render the **existing `controlcenter.html` verbatim** in a **WebKitGTK** webview. A small
  JS shim defines `window.external.invoke(json)` (the bridge the page already uses) to post
  to the Go host, and the host drives the page via `applyState(...)` / `setActive(...)` —
  the identical JSON message protocol the Windows build uses. All current settings/controls
  work unchanged.
- **Tray icon** via StatusNotifierItem / AppIndicator (`libayatana-appindicator3`, confirmed
  present): the mic icon, swapped to the glow variant while dictating. Left-click toggles
  the panel; right-click menu offers Open and Quit.
- **Phase-1 simplification:** the panel opens as a normal small window, not a pixel-perfect
  tray-anchored flyout. Linux/GNOME exposes no reliable tray-icon geometry, and the flyout
  positioning is a Windows-only nicety. Controls and behavior are identical.

### Start-at-login (`install_linux.go`)

Write/remove an XDG autostart entry at `~/.config/autostart/handymute.desktop` (works across
desktops, no root needed). Driven by the existing "Start at login" toggle; `startupEnabled()`
checks for the file.

## Settings & persistence

Unchanged. Existing `settings.go` / `config.go` are shared. Settings continue to persist to
`handymute.conf`. (`cable=` remains a recognized key for Windows; it is simply unused on
Linux.)

## Build & distribution

- `go build` produces the Linux binary. Runtime dependencies — X11, Xtst, WebKitGTK,
  ayatana-appindicator — are present on essentially all desktop Linux installs.
- Add `build_linux.sh` mirroring the existing `build.sh`.
- Broad-distribution packaging (AppImage, as Handy itself ships) is a later step, not part
  of phase 1.

## Testing / verification

- **Audio worker:** unit-test the stream-selection logic (given a list of capture streams,
  the correct non-Handy set is chosen and Handy excluded) with table-driven fixtures.
- **End-to-end (manual, this machine):** with Handy running and a Teams/Discord call,
  hold Ctrl+Space and confirm (a) teammates hear silence/the chosen level, (b) own output
  dims, (c) Handy still dictates, (d) everything restores on release.
- **Regression:** the Windows build must still compile (`GOOS=windows go build`).

## Departures from the upstream README roadmap

Two, both driven by requirement 2 and prior real-world experience, and both flagged for the
maintainer's awareness since they contradict the README's stated Linux plan:

1. No virtual cable on Linux.
2. Stream attenuation instead of per-app device routing.
