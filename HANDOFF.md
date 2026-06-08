# Handoff — Handy Audio Output Blocker (Windows continuation)

**For:** the Claude session running on Austin's Windows machine.
**Date written:** 2026-06-08.
**Status:** Code complete and verified-to-compile (cross-compiled from Linux). **Never run on
Windows yet.** Your job is to get it actually working and fix whatever surfaces at runtime.

---

## 1. What this project does (the goal)

Austin uses **Handy**, a push-to-talk dictation tool bound to **`ctrl+space`**. When he's in a call
(Teams / Zoom / Discord) and holds `ctrl+space` to dictate, the call participants hear his dictation
— which he doesn't want. 

**Goal:** while `ctrl+space` is held, call participants hear **silence**, but **Handy still receives
the mic** so dictation works. On release, participants hear him again.

The hard constraint: one physical mic feeds two consumers (Handy + the call app) with opposite needs
during the hold. You cannot solve this by muting the physical mic — that kills Handy too. The two
consumers must be separated so one can be muted independently.

---

## 2. Two implementations exist in this repo

### Primary: `handymute.exe` (single executable, recommended)
A self-contained Go program. **No VB-Cable, no AutoHotkey, no svcl, no driver.** It:
1. Installs a low-level keyboard hook (`WH_KEYBOARD_LL`) for `ctrl+space`, **passed through** so Handy
   still receives it.
2. On key-down, mutes the **WASAPI per-application capture sessions** of the configured call apps
   (Teams/Zoom/Discord) via `ISimpleAudioVolume.SetMute`. Handy's own session is never matched, so it
   keeps recording.
3. On key-up, unmutes them.

This is the approach to get working first. Why it should work: per-app capture-session mute reaches
apps that capture in shared-mode WASAPI, which Teams/Zoom/Discord do.

### Fallback: VB-Cable + AutoHotkey + svcl
Only if per-app mute fails for some app. Routes the physical mic → `CABLE Input` via Windows "Listen
to this device"; call apps use `CABLE Output`; an AutoHotkey daemon mutes `CABLE Input` via NirSoft
`svcl.exe` on `ctrl+space`. Fully documented in `README-vbcable.md`. The original version of this was
broken (no cable feed, muted the wrong device, never restored default) — the version in the repo is
the corrected one.

---

## 3. Repo layout

```
handyAudioOutputBlocker/
├── HANDOFF.md                  ← this file
├── README.md                   ← single-exe docs (primary)
├── README-vbcable.md           ← VB-Cable fallback docs
├── build.sh                    ← cross-compile script (Linux/macOS); see §6 for Windows build
├── go.mod / go.sum
├── cmd/handymute/
│   ├── main.go                 ← entry point, flag parsing (-install/-uninstall), wires hook→worker
│   ├── hook.go                 ← WH_KEYBOARD_LL keyboard hook + message pump (the ctrl+space logic)
│   ├── audio.go                ← WASAPI: enumerate capture sessions, mute/unmute by process name
│   ├── config.go               ← default process list, optional handymute.conf, logging
│   └── install.go              ← per-user Run registry key for autostart
├── dist/                       ← built binaries (gitignored; rebuild on Windows, see §6)
│   ├── handymute.exe           ← silent build (-H windowsgui), for daily use
│   └── handymute-console.exe   ← console build, prints live logs — USE THIS FOR TESTING
├── docs/superpowers/specs/2026-06-08-handy-mic-windows-design.md   ← design doc
└── handy-mic-*.ahk / *.ps1     ← VB-Cable fallback scripts
```

The `dist/*.exe` binaries are physically present in the repo and were cross-compiled on Linux. They
are **pure-Go native Windows executables with no runtime dependency** (no cgo; they only use
always-present system DLLs like `user32.dll`/`kernel32.dll`). **You can run them as-is on Windows
without installing anything** — start with `dist\handymute-console.exe` (§7). Rebuilding (§6) is only
needed if you change the source.

---

## 4. How the code works (key details for debugging)

- **Threading:** the keyboard hook runs on the main thread (`runtime.LockOSThread` + a `GetMessage`
  pump in `hook.go`). The hook callback must return fast or Windows silently drops the hook, so it
  does **no COM work** — it just sends a `bool` (mute/unmute) over a buffered channel. A separate
  `muteWorker` goroutine (`audio.go`) owns all COM/WASAPI work on its own `LockOSThread`'d thread with
  its own `CoInitializeEx(MTA)`.
- **Transition detection:** `ctrl+space` auto-repeats while held (repeated key-downs). The hook tracks
  a `muted bool` and only sends on the actual press/release transition. Ctrl state is read via
  `GetAsyncKeyState(VK_CONTROL)`.
- **Pass-through:** the hook always calls `CallNextHookEx` and returns its result, so Handy still
  gets `ctrl+space`. It never swallows the key.
- **Mute logic (`audio.go`):** enumerate active capture endpoints → each device's sessions
  (`IAudioSessionManager2` → `IAudioSessionEnumerator`) → resolve owning PID
  (`IAudioSessionControl2.GetProcessId`) → resolve PID to exe name (`QueryFullProcessImageName`) →
  if the lowercased name is in the allowlist, `ISimpleAudioVolume.SetMute(mute)`. Re-enumerated fresh
  on every toggle so it stays correct as sessions come and go.
- **Logging:** every action is logged to stdout **and** to `handymute.log` next to the exe — so even
  the silent build is debuggable. The console build also shows it live.
- **Config:** default process list is in `config.go` (`teams.exe`, `ms-teams.exe`, `zoom.exe`,
  `discord.exe`, `slack.exe`, `webex.exe`, `webexmta.exe`). An optional `handymute.conf` next to the
  exe (one process name per line) **replaces** the defaults.
- **Autostart:** `handymute.exe -install` writes `HKCU\...\CurrentVersion\Run\HandyMute` (no admin).
  `-uninstall` removes it.
- **Dependencies:** `github.com/go-ole/go-ole`, `github.com/moutend/go-wca/pkg/wca`,
  `golang.org/x/sys/windows`. Note: `IAudioSessionControl` → `IAudioSessionControl2` /
  `ISimpleAudioVolume` conversions use go-ole's `PutQueryInterface(IID, &target)`.

---

## 5. What's been verified vs. NOT verified

**Verified (on Linux):**
- Compiles cleanly for `GOOS=windows GOARCH=amd64` (both console and `-H windowsgui` builds).
- `go vet` clean except one expected `unsafe.Pointer` note in `hook.go` — that's the standard,
  correct Win32 hook idiom (the OS guarantees `lParam` points to a live `KBDLLHOOKSTRUCT` during the
  callback). Do **not** "fix" it.
- `gofmt` clean.

**NOT verified (needs you on Windows):**
- That the keyboard hook actually fires and passes `ctrl+space` through to Handy.
- That per-app capture-session mute actually silences Teams/Zoom/Discord (the core risk).
- That Handy keeps transcribing during the hold.
- That `-install` autostart works.
- Latency — whether mute engages fast enough that no speech leaks at the start of the hold.

---

## 6. Building on Windows (optional — only if you change the source)

The prebuilt `dist\*.exe` already run as-is (§3). To rebuild after editing the code, install Go
(https://go.dev/dl/, any recent version), then from the repo root:
```powershell
$env:GOOS="windows"; $env:GOARCH="amd64"
go mod download
go build -trimpath -o dist\handymute-console.exe .\cmd\handymute            # console build (testing)
go build -trimpath -ldflags="-H windowsgui -s -w" -o dist\handymute.exe .\cmd\handymute  # silent build
```
(You can drop the `$env:GOOS/GOARCH` lines when building natively on Windows — they default correctly.)

---

## 7. Test plan (do this first, in order)

1. Join/start a call so Teams/Zoom/Discord is **actively capturing** the mic (a session only exists
   while the app is using the mic).
2. Run `dist\handymute-console.exe` in a terminal. On startup it prints the process list it watches.
3. Hold `ctrl+space`. Expect logs like `Teams.exe pid=1234 -> mute=true` and
   `muted 1 call-app capture session(s)`. Release → `mute=false`.
4. Confirm: participants hear **nothing** during the hold, hear you on release.
5. Confirm: **Handy still transcribes** during the hold.
6. If good: `Ctrl+C`, then `dist\handymute.exe -install` for silent daily use. Watch the silent
   build's log with `Get-Content .\dist\handymute.log -Wait`.

---

## 8. Likely problems & what to do

| Symptom | Cause | Fix |
|---|---|---|
| `muted 0 call-app capture session(s)` | App not capturing yet, or its real `.exe` name isn't in the list | Make sure the call app is live/unmuted; check the actual process name in Task Manager → Details; add it to a `handymute.conf` next to the exe |
| Logs `mute=true` but participants still hear you | That app bypasses per-session mute (exclusive-mode / raw stream) | **Switch to the VB-Cable fallback** in `README-vbcable.md` |
| Handy stops getting `ctrl+space` | Hook is swallowing the key (shouldn't happen — it passes through) | Real bug; check `CallNextHookEx` return path in `hook.go` |
| Hook stops firing after a moment | LL hook callback too slow, Windows dropped it | The design already offloads COM work to a worker; if it recurs, confirm the callback isn't blocking on the channel send (`send()` is non-blocking) |
| Speech leaks at the very start of the hold | Mute latency | Per-app mute should be fast; if not, the VB-Cable fallback (or pre-muting heuristics) may be needed |
| `-install` does nothing at login | Wrong path / quoting in Run key | Inspect `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` value `HandyMute` |

---

## 9. Open design questions you may need to decide with Austin

- **Mute scope:** currently an allowlist of call-app process names. Alternative: mute *all* capture
  sessions except Handy's (more app-agnostic, but riskier — could mute things he wants live like OBS
  or a recorder). Keep the allowlist unless he asks otherwise.
- **Hotkey:** hardcoded to `ctrl+space` (Handy's default). If Austin rebinds Handy, this needs to
  match — consider making it configurable.
- **New Teams process name:** new Teams is `ms-teams.exe`; some builds route audio through
  `msedgewebview2.exe`. If new Teams doesn't mute, try adding `msedgewebview2.exe` to
  `handymute.conf` and check which PID owns the capture session.

---

## 10. Reference

- Full single-exe docs: `README.md`
- VB-Cable fallback docs: `README-vbcable.md`
- Design doc: `docs/superpowers/specs/2026-06-08-handy-mic-windows-design.md`
- This work is also documented in Austin's vault: entity `handy-audio-output-blocker`, concepts
  `per-app-mic-session-mute` and `virtual-cable-mic-isolation`.
