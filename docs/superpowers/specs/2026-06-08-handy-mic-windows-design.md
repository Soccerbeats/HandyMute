# Handy Audio Output Blocker — Windows Design (2026-06-08)

## Goal

While `ctrl+space` (Handy's push-to-talk dictation hotkey) is **held**, call participants in
Teams / Zoom / Discord must hear **silence**. Handy must **always** receive microphone audio,
including during the hold. On release, call participants hear the user again.

## Core constraint

One physical microphone feeds two consumers with opposite requirements during the hold:

- **Handy** — must keep hearing the mic.
- **Call apps** — must stop hearing the mic.

You cannot satisfy both by muting the physical mic. The two consumers must read from
**different devices** so one can be muted without affecting the other.

## Architecture

```
Physical mic ──┬─────────────────────────────────► Handy (reads physical mic directly)
               │
               └─►[Windows "Listen to this device"]─► CABLE Input ─► CABLE Output ─► Teams/Zoom/Discord
                                                          ▲
                                          mute/unmute THIS device on hold/release
```

- **Handy** stays pointed at the real physical microphone — never touched, never muted.
- **Call apps** are pointed at `CABLE Output (VB-Audio Virtual Cable)`.
- A continuous **bridge** routes the physical mic into `CABLE Input` using the built-in Windows
  "Listen to this device" feature (mic → listen-through → `CABLE Input`). Without this bridge the
  cable carries no audio and calls hear silence permanently — this was the missing piece in the
  original implementation.
- On `ctrl+space` **down**, mute `CABLE Input (VB-Audio Virtual Cable)` (the render endpoint feeding
  the cable). The signal is cut at the source, so `CABLE Output` goes silent regardless of how the
  call app handles endpoint mute. On **up**, unmute it.

## Why `svcl.exe`, not nircmd or PowerShell-per-keypress

- `nircmd mutesysvolume` only targets the **default** capture device — it cannot reliably target a
  named device, so it risks muting the real mic (and Handy). The original toggle also permanently
  flipped the default capture device via `setdefaultsounddevice` and never restored it.
- Spawning `powershell.exe` + importing an audio module on every keypress is slow enough (hundreds
  of ms) to leak the first fraction of speech to the call before mute engages.
- **SoundVolumeView's `svcl.exe`** (NirSoft) is a single tiny executable — fast like nircmd, but it
  mutes a device **by name** (`svcl.exe /Mute "CABLE Input (VB-Audio Virtual Cable)"`). AHK calls it
  directly with no PowerShell wrapper, so the mute is effectively instant and device-specific.

## Components

| File | Role |
|---|---|
| `handy-mic-daemon.ahk` | AHK v2 hotkey listener. `~^Space` → `svcl /Mute`, `~^Space up` → `svcl /Unmute`, calling `svcl.exe` directly. `~` passes the key through so Handy still receives it. |
| `handy-mic-toggle.ps1` | Manual mute/unmute helper for testing/verification (and a fallback path). Not on the hot path. |
| `handy-mic-setup.ps1` | Registers the AHK daemon to auto-start at login via Task Scheduler. |
| `handy-mic-rollback.ps1` | Unregisters the task, kills the daemon, and force-unmutes `CABLE Input` as a safety net. |

## Prerequisites (manual installs)

- **VB-Cable** — https://vb-audio.com/Cable/ (install as Administrator; reboot if prompted).
- **AutoHotkey v2** — https://www.autohotkey.com.
- **SoundVolumeView / `svcl.exe`** — https://www.nirsoft.net/utils/sound_volume_command_line.html.

## One-time manual setup (not scriptable)

The "Listen to this device" checkbox is not reliably script-togglable, so it is a documented manual
step: Sound Control Panel → Recording → physical mic → Properties → **Listen** tab → check
**"Listen to this device"** → set **"Playback through this device"** to
`CABLE Input (VB-Audio Virtual Cable)` → Apply. Everything else (mute toggle, autostart, rollback)
is handled by the scripts.

## Device names (exact)

- Render (feed/source, the one we mute): `CABLE Input (VB-Audio Virtual Cable)`
- Capture (what call apps select as mic): `CABLE Output (VB-Audio Virtual Cable)`

## Known tradeoffs / fallbacks

- "Listen to this device" adds ~50–100 ms latency to the normal call audio path — acceptable for
  speech.
- If "Listen" proves flaky on the user's hardware, fall back to **VoiceMeeter** as the mixer
  (near-zero latency, scriptable mute) — same architecture, heavier install.
- If muting the render endpoint (`CABLE Input`) doesn't fully silence a particular app, fall back to
  muting the capture endpoint (`CABLE Output`) instead — one-word change in the scripts.

## Verification (run on Windows)

1. VB-Cable present: `Get-PnpDevice | ? { $_.FriendlyName -like "*VB-Audio*" }`
2. Bridge live: speak → call app's input meter moves.
3. `svcl.exe /Mute "CABLE Input (VB-Audio Virtual Cable)"` → call meter goes silent; `/Unmute` → returns.
4. Hold `ctrl+space` → call participant hears nothing; release → hears you.
5. Handy still transcribes during the hold.
6. Survives reboot (Task Scheduler auto-start).
