# Handy Audio Output Blocker — VB-Cable fallback

> This is the **fallback** approach. Prefer the single-exe `handymute.exe` ([README.md](README.md)).
> Use this only if per-app muting doesn't silence one of your call apps.

Silences outbound audio to call participants (Teams, Zoom, Discord) while `ctrl+space` is held for
Handy dictation, using a virtual audio cable. Handy always keeps mic access.

## How it works

One physical mic can't feed Handy and your call apps with opposite needs during the hold, so they
read from **different** devices:

```
Physical mic ──┬──────────────────────────────────► Handy (reads the physical mic directly)
               │
               └─►[Windows "Listen to this device"]─► CABLE Input ─► CABLE Output ─► Teams/Zoom/Discord
                                                          ▲
                                          muted while ctrl+space is held
```

- **Handy** stays on your real microphone — never muted.
- **Call apps** listen to `CABLE Output`.
- A continuous bridge routes your real mic into `CABLE Input` (Windows "Listen to this device").
- While `ctrl+space` is held, the daemon mutes `CABLE Input`, so the cable — and therefore the call
  apps — goes silent. On release it unmutes. Handy is unaffected throughout.

## Prerequisites

| Tool | Where |
|---|---|
| VB-Cable | https://vb-audio.com/Cable/ (run installer as Administrator) |
| AutoHotkey v2 | https://www.autohotkey.com |
| SoundVolumeView (`svcl.exe`) | https://www.nirsoft.net/utils/sound_volume_command_line.html |

## Setup

1. **Install VB-Cable** — reboot if prompted. Confirm both `CABLE Input (VB-Audio Virtual Cable)`
   (Playback) and `CABLE Output (VB-Audio Virtual Cable)` (Recording) appear in Sound Settings. Do
   NOT make either one your system default device.

2. **Build the bridge (one-time, manual — this is the key step):**
   - Open **Sound Control Panel** (`mmsys.cpl`) → **Recording** tab.
   - Right-click your **physical microphone** → **Properties** → **Listen** tab.
   - Check **"Listen to this device"**.
   - Set **"Playback through this device"** to `CABLE Input (VB-Audio Virtual Cable)`.
   - **Apply**. Your real mic now continuously feeds the cable.

3. **Point call apps at the VB-Cable mic:**
   - Teams: Settings → Devices → Microphone → `CABLE Output (VB-Audio Virtual Cable)`
   - Zoom: Settings → Audio → Microphone → `CABLE Output (VB-Audio Virtual Cable)`
   - Discord: Settings → Voice & Video → Input Device → `CABLE Output (VB-Audio Virtual Cable)`

4. **Leave Handy on your real physical microphone** (its default — do not point Handy at VB-Cable).

5. **Copy files to `C:\Tools\`:**
   ```
   svcl.exe                 (download separately, from SoundVolumeView)
   handy-mic-daemon.ahk
   handy-mic-toggle.ps1
   handy-mic-setup.ps1
   handy-mic-rollback.ps1
   ```

6. **Register the Task Scheduler entry** (run once as Administrator):
   ```powershell
   powershell.exe -ExecutionPolicy Bypass -File "C:\Tools\handy-mic-setup.ps1"
   ```

7. **Verify the task was created:**
   ```powershell
   Get-ScheduledTask -TaskName "HandyMicDaemon"
   ```

## Verification Checklist

1. VB-Cable visible: `Get-PnpDevice | Where-Object { $_.FriendlyName -like "*VB-Audio*" }`
2. Bridge live: speak into your mic → the call app's input meter moves.
3. `svcl.exe` mutes the right device:
   ```powershell
   C:\Tools\svcl.exe /Mute "CABLE Input (VB-Audio Virtual Cable)"     # call meter goes silent
   C:\Tools\svcl.exe /Unmute "CABLE Input (VB-Audio Virtual Cable)"   # call meter returns
   ```
4. Hold `ctrl+space` → call participant hears nothing; release → hears you.
5. Handy still transcribes during the hold.
6. Survives reboot (Task Scheduler auto-start).

## Rollback

```powershell
powershell.exe -ExecutionPolicy Bypass -File "C:\Tools\handy-mic-rollback.ps1"
```

Then, to fully undo: uncheck "Listen to this device" on your mic and reset Teams/Zoom/Discord mic
back to your real microphone.

## Troubleshooting

**Calls hear nothing even when not dictating:** The Listen bridge (Setup step 2) isn't active or
isn't pointed at `CABLE Input`. Re-check the Listen tab on your physical mic.

**A specific app still leaks audio during the hold:** Mute the capture endpoint instead of the
render endpoint — change `CABLE Input` to `CABLE Output (VB-Audio Virtual Cable)` in
`handy-mic-daemon.ahk` (and `handy-mic-toggle.ps1`).

**Latency / echo on calls is annoying:** "Listen to this device" adds ~50-100 ms.

**PowerShell scripts won't run:**
```powershell
Set-ExecutionPolicy -Scope CurrentUser RemoteSigned
```

**Handy stops receiving ctrl+space:** Confirm the `~` prefix is present in the AHK script.
