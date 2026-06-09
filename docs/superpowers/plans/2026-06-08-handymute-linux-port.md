# HandyMute Linux Port Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a native Linux build of HandyMute (X11/GNOME, phase 1) alongside the existing Windows build, sharing all platform-neutral code, mergeable upstream.

**Architecture:** Split platform code via Go filename build constraints (`*_windows.go` / `*_linux.go`). Shared: entry point, settings, config, the Control Center HTML, and the icon-drawing + control-message logic. Linux audio ducks every non-Handy capture stream and the default sink via the PulseAudio protocol (`pactl --format=json`); the hotkey passively reads Ctrl+Space via the X RECORD extension (cgo); the UI reuses `controlcenter.html` in a WebKitGTK webview with an AppIndicator tray, all on one GTK main loop. A fully working headless product exists after Milestone 3; the GTK UI is the final isolated milestone.

**Tech Stack:** Go 1.25, cgo (libX11/libXtst, GTK3, WebKit2GTK-4.1, libayatana-appindicator3), `pactl` (PulseAudio 16 protocol, served by PipeWire), pure-Go `image` for icons.

---

## Conventions for every code step

- All new Linux files start with the build constraint line `//go:build linux` followed by a blank line, then `package main`.
- Run Go commands with the project toolchain: `export GOROOT=$HOME/.goroot PATH=$HOME/.goroot/bin:$PATH` if `go` is not already 1.25 on PATH (`go version` to check).
- The working directory for all commands is the repo root `/home/drawls/DEV/HandyMute`.
- The branch is `linux-port` (already created).

---

## Milestone 1 — Cross-platform build skeleton

Goal: rename Windows files to `*_windows.go`, extract shared code, add compiling Linux stubs, so `go build ./cmd/handymute` succeeds on Linux (producing a binary that runs and logs but does nothing yet) **and** `GOOS=windows GOARCH=amd64 go build ./cmd/handymute` still succeeds.

### Task 1.1: Rename Windows-only files

**Files:** rename within `cmd/handymute/`.

- [ ] **Step 1: Rename via git so history is preserved**

```bash
cd cmd/handymute
git mv hook.go    hook_windows.go
git mv audio.go   audio_windows.go
git mv bridge.go  bridge_windows.go
git mv ui.go      ui_windows.go
git mv icons.go   icons_windows.go
git mv install.go install_windows.go
cd ../..
```

- [ ] **Step 2: Add the build constraint to each renamed file**

At the very top of each of the six `*_windows.go` files, add this as the first two lines (above `package main`):

```go
//go:build windows

```

(The `_windows.go` suffix already constrains the build; the explicit tag makes intent obvious and lets `gofmt`/editors treat it correctly. Do not remove the suffix.)

- [ ] **Step 3: Verify the Windows build still compiles**

Run: `GOOS=windows GOARCH=amd64 go build -o /tmp/hm.exe ./cmd/handymute`
Expected: PASS (no output). This is our Windows regression check; rerun it after every milestone.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor: constrain Windows-only files to GOOS=windows"
```

### Task 1.2: Extract shared `send` helper

`send` lives in `hook_windows.go` but `ui_windows.go` and the Linux UI both need it.

**Files:**
- Modify: `cmd/handymute/hook_windows.go` (remove `send`)
- Modify: `cmd/handymute/config.go` (add `send`)

- [ ] **Step 1: Delete the `send` function from `hook_windows.go`**

Remove these lines (currently the last function in the file):

```go
// send delivers a mute/unmute command without ever blocking the hook callback.
func send(cmd chan<- bool, v bool) {
	select {
	case cmd <- v:
	default:
	}
}
```

- [ ] **Step 2: Add `send` to the shared `config.go`**

Append to `cmd/handymute/config.go`:

```go
// send delivers a mute/unmute command without ever blocking the caller.
func send(cmd chan<- bool, v bool) {
	select {
	case cmd <- v:
	default:
	}
}
```

- [ ] **Step 3: Verify Windows still builds**

Run: `GOOS=windows GOARCH=amd64 go build -o /tmp/hm.exe ./cmd/handymute`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/handymute/config.go cmd/handymute/hook_windows.go
git commit -m "refactor: move send() to shared config.go"
```

### Task 1.3: Extract shared icon drawing

The pure-Go drawing functions (`micCanvas`, `micInside`, `roundRect`, `clampF`, `withHalo`, the `micIdle`/`micGreen` colors, and `iconSize`) are reused by the Linux tray. Only `loadIcons()` is Walk-specific.

**Files:**
- Create: `cmd/handymute/icons_draw.go` (shared, no build tag)
- Modify: `cmd/handymute/icons_windows.go` (keep only `loadIcons`)

- [ ] **Step 1: Create `cmd/handymute/icons_draw.go`**

Move the shared pieces into a new file with NO build constraint:

```go
package main

import (
	"image"
	"image/color"
	"image/draw"
	"math"
)

// Tray-icon colors.
var (
	micIdle  = color.RGBA{R: 230, G: 230, B: 235, A: 255} // neutral mic when not talking
	micGreen = color.RGBA{R: 52, G: 199, B: 89, A: 255}   // iOS green, while actively dictating
)

const iconSize = 44 // canvas px; leaves padding around the 32-ish mic for the glow halo

// micCanvas renders a microphone glyph in col on a transparent iconSize×iconSize canvas,
// anti-aliased via 4x supersampling.
func micCanvas(col color.RGBA) *image.RGBA {
	const ss = 4
	out := image.NewRGBA(image.Rect(0, 0, iconSize, iconSize))
	for oy := 0; oy < iconSize; oy++ {
		for ox := 0; ox < iconSize; ox++ {
			cov := 0
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					fx := float64(ox) + (float64(sx)+0.5)/ss
					fy := float64(oy) + (float64(sy)+0.5)/ss
					if micInside(fx, fy) {
						cov++
					}
				}
			}
			if cov > 0 {
				out.SetRGBA(ox, oy, color.RGBA{col.R, col.G, col.B, uint8(cov * 255 / (ss * ss))})
			}
		}
	}
	return out
}

func micInside(x, y float64) bool {
	if roundRect(x, y, 16, 7, 28, 25, 6) {
		return true
	}
	dx, dy := x-22, y-22
	if d := math.Hypot(dx, dy); d <= 11 && d >= 8 && dy >= -7 {
		return true
	}
	if roundRect(x, y, 21, 33, 23, 38, 1) {
		return true
	}
	if roundRect(x, y, 15, 37.5, 29, 40, 1.2) {
		return true
	}
	return false
}

func roundRect(px, py, x0, y0, x1, y1, r float64) bool {
	if px < x0 || px > x1 || py < y0 || py > y1 {
		return false
	}
	cx := clampF(px, x0+r, x1-r)
	cy := clampF(py, y0+r, y1-r)
	return math.Hypot(px-cx, py-cy) <= r
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// withHalo returns a new image with a soft glow behind the opaque pixels of mic.
func withHalo(mic *image.RGBA, glowColor color.RGBA) *image.RGBA {
	b := mic.Bounds()
	res := image.NewRGBA(b)

	alphaAt := func(x, y int) float64 {
		if x < b.Min.X || x >= b.Max.X || y < b.Min.Y || y >= b.Max.Y {
			return 0
		}
		return float64(mic.RGBAAt(x, y).A) / 255
	}

	const radius = 4
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			var best float64
			for dy := -radius; dy <= radius; dy++ {
				for dx := -radius; dx <= radius; dx++ {
					a := alphaAt(x+dx, y+dy)
					if a <= 0 {
						continue
					}
					fall := 1 - float64(dx*dx+dy*dy)/float64(radius*radius+1)
					if fall > 0 {
						if v := a * fall; v > best {
							best = v
						}
					}
				}
			}
			if best > 0 {
				res.SetRGBA(x, y, color.RGBA{glowColor.R, glowColor.G, glowColor.B, uint8(best * 230)})
			}
		}
	}

	draw.Draw(res, b, mic, b.Min, draw.Over)
	return res
}
```

- [ ] **Step 2: Reduce `icons_windows.go` to only the Walk wrapper**

`icons_windows.go` must now contain ONLY the build tag, package line, the `walk` import, and `loadIcons()`. Its full new contents:

```go
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
```

- [ ] **Step 3: Verify Windows still builds**

Run: `GOOS=windows GOARCH=amd64 go build -o /tmp/hm.exe ./cmd/handymute`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/handymute/icons_draw.go cmd/handymute/icons_windows.go
git commit -m "refactor: extract shared icon drawing into icons_draw.go"
```

### Task 1.4: Linux stub files so the Linux build compiles

Create minimal Linux implementations of the contract `main.go` requires: `muteWorker`, `runHook`, `runUI`, `setupBridge`, `removeBridge`, `installStartup`, `uninstallStartup`, `startupEnabled`. Real logic comes in later milestones.

**Files:**
- Create: `cmd/handymute/bridge_linux.go`
- Create: `cmd/handymute/audio_linux.go`
- Create: `cmd/handymute/hook_linux.go`
- Create: `cmd/handymute/install_linux.go`
- Create: `cmd/handymute/ui_linux.go`

- [ ] **Step 1: `bridge_linux.go` — no virtual cable on Linux**

```go
//go:build linux

package main

import "errors"

// Linux uses no virtual audio cable, so the bridge flags are not supported.
func setupBridge() error  { return errors.New("-setup-bridge is Windows-only; not needed on Linux") }
func removeBridge() error { return errors.New("-remove-bridge is Windows-only; not needed on Linux") }
```

- [ ] **Step 2: `audio_linux.go` — stub worker**

```go
//go:build linux

package main

// muteWorker applies/reverses the duck on each hold transition. Real implementation lands in
// Milestone 2.
func muteWorker(cmd <-chan bool, settings *Settings) {
	for range cmd {
	}
}
```

- [ ] **Step 3: `hook_linux.go` — stub hook**

```go
//go:build linux

package main

// runHook watches Ctrl+Space. Real implementation (X RECORD) lands in Milestone 3.
func runHook(cmd chan<- bool, status chan<- bool, settings *Settings) error {
	select {} // block forever
}
```

- [ ] **Step 4: `install_linux.go` — stub startup**

```go
//go:build linux

package main

func startupEnabled() bool      { return false }
func installStartup() error     { return nil }
func uninstallStartup() error   { return nil }
```

- [ ] **Step 5: `ui_linux.go` — stub UI that keeps the process alive**

```go
//go:build linux

package main

// runUI hosts the tray + Control Center. Real implementation lands in Milestone 4.
func runUI(settings *Settings, cmd chan<- bool, status <-chan bool) error {
	logf("handymute (linux) running — UI not yet implemented; press Ctrl+C to quit")
	for range status { // drain status so the glow goroutine never blocks
	}
	select {}
}
```

- [ ] **Step 6: Verify the Linux build compiles and runs**

Run: `go build -o /tmp/handymute ./cmd/handymute && timeout 2 /tmp/handymute; echo "exit=$?"`
Expected: prints the `handymute starting...` log line and the "UI not yet implemented" line, then exits via timeout (`exit=124`).

- [ ] **Step 7: Verify Windows still builds**

Run: `GOOS=windows GOARCH=amd64 go build -o /tmp/hm.exe ./cmd/handymute`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add cmd/handymute/bridge_linux.go cmd/handymute/audio_linux.go cmd/handymute/hook_linux.go cmd/handymute/install_linux.go cmd/handymute/ui_linux.go
git commit -m "feat(linux): compiling stub platform layer"
```

---

## Milestone 2 — Audio worker (zero-config ducking)

Goal: `muteWorker` on Linux, on hold, silences every non-Handy capture stream to `teams_level` and ducks the default sink to `speaker_duck`; restores exact prior volumes on release. The stream-selection and JSON parsing are pure functions developed with TDD.

### Task 2.1: Capture-stream JSON parsing (TDD)

**Files:**
- Create: `cmd/handymute/audio_linux_parse.go`
- Test: `cmd/handymute/audio_linux_parse_test.go`

- [ ] **Step 1: Write the failing test**

`cmd/handymute/audio_linux_parse_test.go`:

```go
//go:build linux

package main

import "testing"

// Real pactl --format=json shape, trimmed to the fields we use. Includes Teams (a call app),
// Handy (must be excluded), and a pavucontrol peak meter (harmless to duck).
const sampleSourceOutputs = `[
  {"index":104718,"properties":{"application.name":"PulseAudio Volume Control","application.process.binary":"pavucontrol","node.name":"PulseAudio Volume Control"},"volume":{"mono":{"value_percent":"100%"}}},
  {"index":127909,"properties":{"application.name":"Teams","application.process.binary":"chrome","node.name":"Teams"},"volume":{"front-left":{"value_percent":"100%"},"front-right":{"value_percent":"100%"}}},
  {"index":200001,"properties":{"application.name":"Handy","application.process.binary":"handy","node.name":"Handy"},"volume":{"mono":{"value_percent":"90%"}}}
]`

func TestParseCaptureStreams(t *testing.T) {
	got, err := parseCaptureStreams([]byte(sampleSourceOutputs))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 streams, got %d", len(got))
	}
	teams := got[1]
	if teams.Index != 127909 || teams.AppName != "Teams" || teams.Binary != "chrome" || teams.VolumePercent != 100 {
		t.Errorf("unexpected Teams parse: %+v", teams)
	}
	if got[2].VolumePercent != 90 {
		t.Errorf("want Handy 90%%, got %d", got[2].VolumePercent)
	}
}

func TestIsHandy(t *testing.T) {
	cases := []struct {
		cs   captureStream
		want bool
	}{
		{captureStream{Binary: "handy"}, true},
		{captureStream{AppName: "Handy"}, true},
		{captureStream{NodeName: "handy_capture"}, true},
		{captureStream{Binary: "chrome", AppName: "Teams"}, false},
		{captureStream{Binary: "pavucontrol"}, false},
	}
	for _, c := range cases {
		if got := isHandy(c.cs); got != c.want {
			t.Errorf("isHandy(%+v) = %v, want %v", c.cs, got, c.want)
		}
	}
}

func TestSelectTargets(t *testing.T) {
	streams, _ := parseCaptureStreams([]byte(sampleSourceOutputs))
	targets := selectTargets(streams)
	if len(targets) != 2 {
		t.Fatalf("want 2 targets (pavucontrol + Teams), got %d", len(targets))
	}
	for _, tg := range targets {
		if isHandy(tg) {
			t.Errorf("Handy must never be a target: %+v", tg)
		}
	}
}

func TestParsePercent(t *testing.T) {
	for in, want := range map[string]int{"100%": 100, "0%": 0, "115%": 115, "90%": 90} {
		if got, ok := parsePercent(in); !ok || got != want {
			t.Errorf("parsePercent(%q) = %d,%v want %d", in, got, ok, want)
		}
	}
	if _, ok := parsePercent("nope"); ok {
		t.Errorf("parsePercent(nope) should fail")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/handymute/ -run 'CaptureStreams|IsHandy|SelectTargets|ParsePercent' -v`
Expected: FAIL — `undefined: parseCaptureStreams` (and the other symbols).

- [ ] **Step 3: Write the implementation**

`cmd/handymute/audio_linux_parse.go`:

```go
//go:build linux

package main

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// captureStream is one application reading a microphone (a PulseAudio source-output).
type captureStream struct {
	Index         int
	Binary        string
	AppName       string
	NodeName      string
	VolumePercent int // representative channel volume, 0..N
}

// parseCaptureStreams decodes `pactl --format=json list source-outputs`.
func parseCaptureStreams(data []byte) ([]captureStream, error) {
	var raw []struct {
		Index      int               `json:"index"`
		Properties map[string]string `json:"properties"`
		Volume     map[string]struct {
			ValuePercent string `json:"value_percent"`
		} `json:"volume"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	out := make([]captureStream, 0, len(raw))
	for _, r := range raw {
		cs := captureStream{
			Index:    r.Index,
			Binary:   r.Properties["application.process.binary"],
			AppName:  r.Properties["application.name"],
			NodeName: r.Properties["node.name"],
		}
		// Representative volume: the first channel in sorted key order (deterministic).
		keys := make([]string, 0, len(r.Volume))
		for k := range r.Volume {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) > 0 {
			if p, ok := parsePercent(r.Volume[keys[0]].ValuePercent); ok {
				cs.VolumePercent = p
			}
		}
		out = append(out, cs)
	}
	return out, nil
}

// isHandy reports whether a capture stream belongs to Handy (which must never be ducked).
func isHandy(cs captureStream) bool {
	if strings.EqualFold(cs.Binary, "handy") {
		return true
	}
	return strings.Contains(strings.ToLower(cs.AppName), "handy") ||
		strings.Contains(strings.ToLower(cs.NodeName), "handy")
}

// selectTargets returns the capture streams to duck (everything except Handy).
func selectTargets(streams []captureStream) []captureStream {
	out := make([]captureStream, 0, len(streams))
	for _, cs := range streams {
		if !isHandy(cs) {
			out = append(out, cs)
		}
	}
	return out
}

// parsePercent turns "100%" into 100.
func parsePercent(s string) (int, bool) {
	s = strings.TrimSuffix(strings.TrimSpace(s), "%")
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/handymute/ -run 'CaptureStreams|IsHandy|SelectTargets|ParsePercent' -v`
Expected: PASS (all four tests).

- [ ] **Step 5: Commit**

```bash
git add cmd/handymute/audio_linux_parse.go cmd/handymute/audio_linux_parse_test.go
git commit -m "feat(linux): capture-stream parsing + non-Handy selection"
```

### Task 2.2: Sink-volume JSON parsing (TDD)

**Files:**
- Modify: `cmd/handymute/audio_linux_parse.go`
- Modify: `cmd/handymute/audio_linux_parse_test.go`

- [ ] **Step 1: Add the failing test**

Append to `audio_linux_parse_test.go`:

```go
const sampleSinks = `[
  {"name":"alsa_output.pci-0000_00_1f.3.analog-stereo","volume":{"front-left":{"value_percent":"115%"},"front-right":{"value_percent":"115%"}}},
  {"name":"some_other_sink","volume":{"mono":{"value_percent":"40%"}}}
]`

func TestSinkVolume(t *testing.T) {
	got, err := parseSinkVolume([]byte(sampleSinks), "alsa_output.pci-0000_00_1f.3.analog-stereo")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != 115 {
		t.Errorf("want 115, got %d", got)
	}
	if _, err := parseSinkVolume([]byte(sampleSinks), "missing"); err == nil {
		t.Errorf("expected error for missing sink")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/handymute/ -run TestSinkVolume -v`
Expected: FAIL — `undefined: parseSinkVolume`.

- [ ] **Step 3: Implement `parseSinkVolume`**

Append to `audio_linux_parse.go`:

```go
import "fmt" // add to the existing import block instead of a second block

// parseSinkVolume decodes `pactl --format=json list sinks` and returns the representative
// volume percent of the sink named want.
func parseSinkVolume(data []byte, want string) (int, error) {
	var raw []struct {
		Name   string `json:"name"`
		Volume map[string]struct {
			ValuePercent string `json:"value_percent"`
		} `json:"volume"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return 0, err
	}
	for _, s := range raw {
		if s.Name != want {
			continue
		}
		keys := make([]string, 0, len(s.Volume))
		for k := range s.Volume {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) > 0 {
			if p, ok := parsePercent(s.Volume[keys[0]].ValuePercent); ok {
				return p, nil
			}
		}
		return 0, fmt.Errorf("sink %q has no readable volume", want)
	}
	return 0, fmt.Errorf("sink %q not found", want)
}
```

NOTE: merge `"fmt"` into the existing `import (...)` block at the top of the file rather than adding a new `import` statement.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/handymute/ -run TestSinkVolume -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/handymute/audio_linux_parse.go cmd/handymute/audio_linux_parse_test.go
git commit -m "feat(linux): parse default-sink volume from pactl json"
```

### Task 2.3: pactl command wrappers + real `muteWorker`

This task is integration code (shells out to `pactl`); verified by build + a live manual check, not unit tests.

**Files:**
- Modify: `cmd/handymute/audio_linux.go` (replace the stub)

- [ ] **Step 1: Replace `audio_linux.go` with the real worker**

```go
//go:build linux

package main

import (
	"os/exec"
	"strconv"
)

// pactlJSON runs `pactl --format=json <args...>` and returns stdout.
func pactlJSON(args ...string) ([]byte, error) {
	return exec.Command("pactl", append([]string{"--format=json"}, args...)...).Output()
}

func listCaptureStreams() ([]captureStream, error) {
	out, err := pactlJSON("list", "source-outputs")
	if err != nil {
		return nil, err
	}
	return parseCaptureStreams(out)
}

func defaultSink() (string, error) {
	out, err := exec.Command("pactl", "get-default-sink").Output()
	if err != nil {
		return "", err
	}
	return string(trimNewline(out)), nil
}

func sinkVolumePercent(name string) (int, error) {
	out, err := pactlJSON("list", "sinks")
	if err != nil {
		return 0, err
	}
	return parseSinkVolume(out, name)
}

func setSourceOutputVolume(index, percent int) {
	_ = exec.Command("pactl", "set-source-output-volume", strconv.Itoa(index), strconv.Itoa(percent)+"%").Run()
}

func setSinkVolume(name string, percent int) {
	_ = exec.Command("pactl", "set-sink-volume", name, strconv.Itoa(percent)+"%").Run()
}

func trimNewline(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

// pct converts a 0..1 scalar to a rounded 0..100 percent.
func pctScalar(v float32) int { return int(v*100 + 0.5) }

// muteWorker owns all audio state. On hold it saves and lowers every non-Handy capture
// stream to teams_level and the default sink to speaker_duck; on release it restores the
// exact saved values. Single goroutine → no locking beyond Settings' own.
func muteWorker(cmd <-chan bool, settings *Settings) {
	savedStreams := map[int]int{} // source-output index -> prior percent
	savedSink := -1               // prior default-sink percent, -1 = nothing saved
	savedSinkName := ""

	for hold := range cmd {
		snap := settings.Snapshot()
		if hold {
			streams, err := listCaptureStreams()
			if err != nil {
				logf("listCaptureStreams: %v", err)
			}
			for _, cs := range selectTargets(streams) {
				savedStreams[cs.Index] = cs.VolumePercent
				setSourceOutputVolume(cs.Index, pctScalar(snap.TeamsLevel))
			}
			if name, err := defaultSink(); err == nil {
				if v, err := sinkVolumePercent(name); err == nil {
					savedSink, savedSinkName = v, name
					setSinkVolume(name, pctScalar(snap.SpeakerDuck))
				}
			}
			logf("hold: ducked %d capture stream(s), sink %q %d%%->%d%%",
				len(savedStreams), savedSinkName, savedSink, pctScalar(snap.SpeakerDuck))
		} else {
			for idx, vol := range savedStreams {
				setSourceOutputVolume(idx, vol)
				delete(savedStreams, idx)
			}
			if savedSink >= 0 {
				setSinkVolume(savedSinkName, savedSink)
				savedSink, savedSinkName = -1, ""
			}
			logf("release: restored")
		}
	}
}
```

- [ ] **Step 2: Verify both builds compile**

Run: `go build -o /tmp/handymute ./cmd/handymute && GOOS=windows GOARCH=amd64 go build -o /tmp/hm.exe ./cmd/handymute`
Expected: PASS (both).

- [ ] **Step 3: Confirm the data the worker will act on**

The worker is exercised end-to-end in Milestone 3 (once the hotkey drives it). For now, confirm `pactl` is callable and a capture stream is visible:

Run: `pactl --format=json list source-outputs | python3 -c "import sys,json;[print(s['index'],s['properties'].get('application.name')) for s in json.load(sys.stdin)]"`
Expected: lists current capture streams (e.g. a Teams entry) — the input the worker selects from.

- [ ] **Step 4: Commit**

```bash
git add cmd/handymute/audio_linux.go
git commit -m "feat(linux): pactl-backed mute worker (duck non-Handy capture + default sink)"
```

---

## Milestone 3 — Hotkey via X RECORD → working headless product

Goal: Ctrl+Space (held) triggers the worker; release restores. After this milestone, running the binary with Handy + a call gives the full mute/duck behavior, no UI required.

### Task 3.1: Real `runHook` with cgo X RECORD

Integration code; verified by build + live end-to-end test.

**Files:**
- Modify: `cmd/handymute/hook_linux.go` (replace the stub)

- [ ] **Step 1: Replace `hook_linux.go`**

```go
//go:build linux

package main

/*
#cgo pkg-config: x11 xtst
#include <X11/Xlib.h>
#include <X11/extensions/record.h>
#include <stdlib.h>

extern void goRecordKey(int type, int keycode, int state);

// recordCallback receives raw protocol bytes for each KeyPress/KeyRelease.
static void recordCallback(XPointer closure, XRecordInterceptData *d) {
    if (d->category == XRecordFromServer && d->data_len * 4 >= 30) {
        unsigned char *p = (unsigned char*)d->data;
        int type    = p[0] & 0x7f;          // 2 = KeyPress, 3 = KeyRelease
        int keycode = p[1];
        int state   = p[28] | (p[29] << 8); // modifier mask at event time (ControlMask = 0x4)
        goRecordKey(type, keycode, state);
    }
    XRecordFreeData(d);
}

// runRecord opens two X connections (control + data) and blocks delivering key events.
static int runRecord() {
    Display *ctrlDpy = XOpenDisplay(NULL);
    Display *dataDpy = XOpenDisplay(NULL);
    if (!ctrlDpy || !dataDpy) return 1;

    XRecordRange *rr = XRecordAllocRange();
    if (!rr) return 2;
    rr->device_events.first = KeyPress;
    rr->device_events.last  = KeyRelease;

    XRecordClientSpec clients = XRecordAllClients;
    XRecordContext ctx = XRecordCreateContext(ctrlDpy, 0, &clients, 1, &rr, 1);
    XFree(rr);
    XSync(ctrlDpy, False);

    if (!XRecordEnableContext(dataDpy, ctx, recordCallback, NULL)) return 3;
    return 0;
}

// spaceKeycode resolves the keycode for the space keysym.
static int spaceKeycode() {
    Display *d = XOpenDisplay(NULL);
    if (!d) return 0;
    int kc = XKeysymToKeycode(d, XStringToKeysym("space"));
    XCloseDisplay(d);
    return kc;
}
*/
import "C"

import (
	"fmt"
	"runtime"
)

const controlMask = 0x04 // X11 ControlMask

// Hook singleton state (the C callback has no user-data path to Go, so we use package vars).
var (
	hookCmd      chan<- bool
	hookStatus   chan<- bool
	hookSettings *Settings
	hookSpaceKC  int
	hookHeld     bool
)

//export goRecordKey
func goRecordKey(typ, keycode, state C.int) {
	if int(keycode) != hookSpaceKC {
		return
	}
	isPress := int(typ) == 2 // KeyPress
	ctrl := int(state)&controlMask != 0

	if isPress && ctrl && !hookHeld && hookSettings.Enabled() {
		hookHeld = true
		send(hookCmd, true)
		send(hookStatus, true)
	} else if !isPress && hookHeld {
		hookHeld = false
		send(hookCmd, false)
		send(hookStatus, false)
	}
}

// runHook passively watches Ctrl+Space via the X RECORD extension. Ctrl+Space is never
// grabbed, so Handy still receives it. Ctrl is read from each space event's modifier state
// field (Handy grabs Ctrl first, so tracking the Ctrl keycode directly is unreliable).
func runHook(cmd chan<- bool, status chan<- bool, settings *Settings) error {
	runtime.LockOSThread()
	hookCmd, hookStatus, hookSettings = cmd, status, settings

	hookSpaceKC = int(C.spaceKeycode())
	if hookSpaceKC == 0 {
		return fmt.Errorf("could not resolve space keycode (no X display?)")
	}
	logf("hook: watching ctrl+space via XRecord (space keycode=%d)", hookSpaceKC)

	if rc := int(C.runRecord()); rc != 0 {
		return fmt.Errorf("XRecord setup failed (code=%d)", rc)
	}
	return fmt.Errorf("XRecord context ended")
}
```

- [ ] **Step 2: Verify both builds compile**

Run: `go build -o /tmp/handymute ./cmd/handymute && GOOS=windows GOARCH=amd64 go build -o /tmp/hm.exe ./cmd/handymute`
Expected: PASS. (If the Linux build errors with a missing `X11/extensions/record.h`, install headers: `echo drawls | sudo -S apt-get install -y libxtst-dev libx11-dev` — though pkg-config confirmed both are present.)

- [ ] **Step 3: Live end-to-end test (headless core)**

Pre-req: Handy is running; have a Teams/Discord call open (or at least Chrome capturing the mic). Run the binary in the foreground:

Run: `/tmp/handymute`
Then hold **Ctrl+Space** for ~2 seconds and release. Observe the log lines:
Expected: on press, `hold: ducked N capture stream(s), sink ... ->20%`; on release, `release: restored`. While held, a teammate hears silence and your own output dims; Handy still transcribes. Ctrl+C to quit.

- [ ] **Step 4: Verify restore correctness**

After the test, run: `pactl --format=json list source-outputs | python3 -c "import sys,json;[print(s['index'],s['properties'].get('application.name'),s['volume'][list(s['volume'])[0]]['value_percent']) for s in json.load(sys.stdin)]"`
Expected: the call app's capture volume is back at its original percent (e.g. 100%), confirming restore.

- [ ] **Step 5: Commit**

```bash
git add cmd/handymute/hook_linux.go
git commit -m "feat(linux): ctrl+space hotkey via X RECORD; headless mute/duck works"
```

---

## Milestone 4 — UI: tray + Control Center

Goal: AppIndicator tray icon (glows while dictating) and the existing `controlcenter.html` in a WebKitGTK window, on one GTK main loop, with the existing JSON bridge.

### Task 4.1: Install the AppIndicator dev headers

- [ ] **Step 1: Install the build dependency**

Run: `echo drawls | sudo -S apt-get install -y libayatana-appindicator3-dev`
Expected: installs the package (runtime `.so` is already present).

- [ ] **Step 2: Confirm pkg-config now resolves it**

Run: `pkg-config --exists ayatana-appindicator3-0.1 && echo OK`
Expected: `OK`.

### Task 4.2: Shared control-message handling (DRY across platforms)

Extract the Control Center message switch (currently in `ui_windows.go`'s `onMessage` / `pushState`) into a shared file both UIs call.

**Files:**
- Create: `cmd/handymute/controlcenter.go`
- Modify: `cmd/handymute/ui_windows.go` (delegate to the shared functions)

- [ ] **Step 1: Create `cmd/handymute/controlcenter.go`**

```go
package main

import "encoding/json"

// handleControlMessage processes one JSON command from the Control Center page. eval runs a
// JS snippet in the page; setGlow swaps the tray glow; cmd signals the audio worker.
func handleControlMessage(raw string, settings *Settings, cmd chan<- bool, eval func(js string), setGlow func(bool)) {
	var m struct {
		Action string          `json:"action"`
		Value  json.RawMessage `json:"value"`
	}
	if json.Unmarshal([]byte(raw), &m) != nil {
		return
	}
	switch m.Action {
	case "ready":
		pushControlState(settings, eval)
	case "enabled":
		var on bool
		json.Unmarshal(m.Value, &on)
		settings.SetEnabled(on)
		if !on {
			send(cmd, false)
			setGlow(false)
		}
	case "teams":
		var v int
		json.Unmarshal(m.Value, &v)
		settings.SetTeamsLevel(float32(v) / 100)
	case "speaker":
		var v int
		json.Unmarshal(m.Value, &v)
		settings.SetSpeakerDuck(float32(v) / 100)
	case "startup":
		var on bool
		json.Unmarshal(m.Value, &on)
		var err error
		if on {
			err = installStartup()
		} else {
			err = uninstallStartup()
		}
		if err != nil {
			logf("start-at-login toggle failed: %v", err)
		}
	case "theme":
		var t string
		json.Unmarshal(m.Value, &t)
		settings.SetTheme(t)
	}
}

// pushControlState sends current settings to the page so controls reflect reality.
func pushControlState(settings *Settings, eval func(js string)) {
	state := map[string]any{
		"enabled": settings.Enabled(),
		"teams":   pct(settings.TeamsLevel()),
		"speaker": pct(settings.SpeakerDuck()),
		"startup": startupEnabled(),
		"theme":   settings.Theme(),
	}
	b, err := json.Marshal(state)
	if err != nil {
		return
	}
	eval("applyState(" + string(b) + ")")
}
```

NOTE: `pct` already exists in `ui_windows.go`. To avoid a duplicate symbol, MOVE `pct` from `ui_windows.go` into this shared file. Delete the `pct` function (and its comment) from `ui_windows.go` in the next step. The Linux `audio_linux.go` defines `pctScalar` separately, so there is no clash.

- [ ] **Step 2: Update `ui_windows.go` to delegate**

In `ui_windows.go`: (a) delete the `pct` function (moved to `controlcenter.go`); (b) replace the bodies of `onMessage` and `pushState` with delegations. The shared handler covers every action except the two Windows-only window-management actions (`hide`, `quit`), which stay here. Replace the whole `onMessage` method with:

```go
func (u *ui) onMessage(raw string) {
	// Windows-only window actions; everything else goes to the shared handler.
	var m struct {
		Action string `json:"action"`
	}
	if json.Unmarshal([]byte(raw), &m) == nil {
		switch m.Action {
		case "hide":
			u.mw.Hide()
			return
		case "quit":
			walk.App().Exit(0)
			return
		}
	}
	handleControlMessage(raw, u.settings, u.cmd,
		func(js string) {
			if u.web != nil {
				u.web.Eval(js)
			}
		},
		u.setGlow,
	)
}
```

And replace the whole `pushState` method with:

```go
func (u *ui) pushState() {
	pushControlState(u.settings, func(js string) {
		if u.web != nil {
			u.web.Eval(js)
		}
	})
}
```

Add `"encoding/json"` to `ui_windows.go`'s imports if not already present (it is).

- [ ] **Step 3: Verify the Windows build still compiles**

Run: `GOOS=windows GOARCH=amd64 go build -o /tmp/hm.exe ./cmd/handymute`
Expected: PASS. (This proves the DRY refactor didn't break Windows.)

- [ ] **Step 4: Verify the Linux build still compiles**

Run: `go build -o /tmp/handymute ./cmd/handymute`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/handymute/controlcenter.go cmd/handymute/ui_windows.go
git commit -m "refactor: share Control Center message handling across platforms"
```

### Task 4.3: Linux tray icons as PNG files

**Files:**
- Create: `cmd/handymute/icons_linux.go`

- [ ] **Step 1: Create `icons_linux.go`**

```go
//go:build linux

package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
)

// writeTrayIcons renders the normal and glow mic icons to PNG files in a temp dir and returns
// the dir. AppIndicator looks up icons by name within an icon theme path, so the files are
// named "handymute.png" and "handymute-active.png".
func writeTrayIcons() (dir string, err error) {
	dir, err = os.MkdirTemp("", "handymute-icons-")
	if err != nil {
		return "", err
	}
	if err := writePNG(filepath.Join(dir, "handymute.png"), micCanvas(micIdle)); err != nil {
		return "", err
	}
	if err := writePNG(filepath.Join(dir, "handymute-active.png"), withHalo(micCanvas(micGreen), micGreen)); err != nil {
		return "", err
	}
	return dir, nil
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build -o /tmp/handymute ./cmd/handymute`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add cmd/handymute/icons_linux.go
git commit -m "feat(linux): render tray icons to PNG for AppIndicator"
```

### Task 4.4: Real `ui_linux.go` — GTK + WebKit + AppIndicator

Integration code; verified by build + live run.

**Files:**
- Modify: `cmd/handymute/ui_linux.go` (replace the stub)

- [ ] **Step 1: Replace `ui_linux.go`**

```go
//go:build linux

package main

/*
#cgo pkg-config: gtk+-3.0 webkit2gtk-4.1 ayatana-appindicator3-0.1
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>
#include <libayatana-appindicator/app-indicator.h>
#include <stdlib.h>
#include <string.h>

extern void goWebMessage(char *msg);
extern void goMenuOpen();
extern void goMenuQuit();

static GtkWidget   *win;
static WebKitWebView *webview;
static AppIndicator *indicator;

// JS -> Go bridge.
static void on_script_message(WebKitUserContentManager *m, WebKitJavascriptResult *r, gpointer u) {
    JSCValue *val = webkit_javascript_result_get_js_value(r);
    char *s = jsc_value_to_string(val);
    goWebMessage(s);
    g_free(s);
}

static gboolean run_js_idle(gpointer data) {
    char *js = (char*)data;
    webkit_web_view_evaluate_javascript(webview, js, -1, NULL, NULL, NULL, NULL, NULL);
    free(js);
    return G_SOURCE_REMOVE;
}
// ui_eval: callable from any goroutine; runs JS on the GTK thread.
static void ui_eval(const char *js) { g_idle_add(run_js_idle, strdup(js)); }

static gboolean show_win_idle(gpointer data) {
    gtk_widget_show_all(win);
    gtk_window_present(GTK_WINDOW(win));
    return G_SOURCE_REMOVE;
}
static void ui_show() { g_idle_add(show_win_idle, NULL); }

static gboolean hide_win_idle(gpointer data) { gtk_widget_hide(win); return G_SOURCE_REMOVE; }
static void ui_hide() { g_idle_add(hide_win_idle, NULL); }

static gboolean set_icon_idle(gpointer data) {
    char *name = (char*)data;
    app_indicator_set_icon_full(indicator, name, "HandyMute");
    free(name);
    return G_SOURCE_REMOVE;
}
static void ui_set_icon(const char *name) { g_idle_add(set_icon_idle, strdup(name)); }

static void menu_open_cb(GtkMenuItem *i, gpointer d) { goMenuOpen(); }
static void menu_quit_cb(GtkMenuItem *i, gpointer d) { goMenuQuit(); }

static gboolean on_delete(GtkWidget *w, GdkEvent *e, gpointer d) {
    gtk_widget_hide(w);  // hide instead of destroy, like a flyout
    return TRUE;
}

static void ui_run(const char *html, const char *iconThemePath) {
    gtk_init(NULL, NULL);

    win = gtk_window_new(GTK_WINDOW_TOPLEVEL);
    gtk_window_set_title(GTK_WINDOW(win), "HandyMute");
    gtk_window_set_default_size(GTK_WINDOW(win), 340, 470);
    gtk_window_set_resizable(GTK_WINDOW(win), FALSE);
    g_signal_connect(win, "delete-event", G_CALLBACK(on_delete), NULL);

    WebKitUserContentManager *ucm = webkit_user_content_manager_new();
    webkit_user_content_manager_register_script_message_handler(ucm, "handymute");
    g_signal_connect(ucm, "script-message-received::handymute", G_CALLBACK(on_script_message), NULL);

    // Shim so the page's existing window.external.invoke(...) reaches our handler unchanged.
    const char *shim =
        "window.external={invoke:function(s){window.webkit.messageHandlers.handymute.postMessage(s);}};";
    WebKitUserScript *us = webkit_user_script_new(
        shim, WEBKIT_USER_CONTENT_INJECT_TOP_FRAME,
        WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START, NULL, NULL);
    webkit_user_content_manager_add_script(ucm, us);

    webview = WEBKIT_WEB_VIEW(webkit_web_view_new_with_user_content_manager(ucm));
    webkit_web_view_load_html(webview, html, NULL);
    gtk_container_add(GTK_CONTAINER(win), GTK_WIDGET(webview));

    indicator = app_indicator_new("handymute", "handymute", APP_INDICATOR_CATEGORY_APPLICATION_STATUS);
    app_indicator_set_icon_theme_path(indicator, iconThemePath);
    app_indicator_set_status(indicator, APP_INDICATOR_STATUS_ACTIVE);

    GtkWidget *menu = gtk_menu_new();
    GtkWidget *mi_open = gtk_menu_item_new_with_label("Open HandyMute");
    g_signal_connect(mi_open, "activate", G_CALLBACK(menu_open_cb), NULL);
    gtk_menu_shell_append(GTK_MENU_SHELL(menu), mi_open);
    GtkWidget *mi_quit = gtk_menu_item_new_with_label("Quit");
    g_signal_connect(mi_quit, "activate", G_CALLBACK(menu_quit_cb), NULL);
    gtk_menu_shell_append(GTK_MENU_SHELL(menu), mi_quit);
    gtk_widget_show_all(menu);
    app_indicator_set_menu(indicator, GTK_MENU(menu));
    app_indicator_set_secondary_activate_target(indicator, mi_open);

    gtk_main();
}
*/
import "C"

import (
	"encoding/json"
	"runtime"
	"unsafe"
)

var (
	uiSettings *Settings
	uiCmd      chan<- bool
)

//export goWebMessage
func goWebMessage(msg *C.char) {
	raw := C.GoString(msg)
	// Window-only actions; everything else goes to the shared handler.
	var m struct {
		Action string `json:"action"`
	}
	if json.Unmarshal([]byte(raw), &m) == nil {
		switch m.Action {
		case "hide":
			C.ui_hide()
			return
		case "quit":
			C.gtk_main_quit()
			return
		}
	}
	handleControlMessage(raw, uiSettings, uiCmd, uiEval, setLinuxGlow)
}

//export goMenuOpen
func goMenuOpen() { C.ui_show() }

//export goMenuQuit
func goMenuQuit() { C.gtk_main_quit() }

// uiEval runs JS in the page from any goroutine.
func uiEval(js string) {
	cjs := C.CString(js)
	defer C.free(unsafe.Pointer(cjs))
	C.ui_eval(cjs)
}

// setLinuxGlow swaps the tray icon and updates the page's status dot.
func setLinuxGlow(on bool) {
	name := "handymute"
	if on {
		name = "handymute-active"
	}
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	C.ui_set_icon(cname)
}

func runUI(settings *Settings, cmd chan<- bool, status <-chan bool) error {
	runtime.LockOSThread()
	uiSettings, uiCmd = settings, cmd

	iconDir, err := writeTrayIcons()
	if err != nil {
		logf("writeTrayIcons: %v", err)
	}

	// Drive the glow (tray icon + page dot) on each hold transition.
	go func() {
		for active := range status {
			setLinuxGlow(active)
			if active {
				uiEval("setActive(true)")
			} else {
				uiEval("setActive(false)")
			}
		}
	}()

	chtml := C.CString(controlCenterHTML)
	cdir := C.CString(iconDir)
	defer C.free(unsafe.Pointer(chtml))
	defer C.free(unsafe.Pointer(cdir))
	C.ui_run(chtml, cdir) // blocks in gtk_main
	return nil
}
```

NOTE: `controlCenterHTML` is the `//go:embed controlcenter.html` variable currently declared in `ui_windows.go`. Since that file is Windows-only, the embed must be shared. Handle this in the next step.

- [ ] **Step 2: Move the `controlcenter.html` embed to a shared file**

Remove the embed declaration from `ui_windows.go`:

```go
//go:embed controlcenter.html
var controlCenterHTML string
```

and remove the now-unused `_ "embed"` import from `ui_windows.go` ONLY IF it is otherwise unused (it is used solely for this embed, so remove it). Create `cmd/handymute/controlcenter_html.go`:

```go
package main

import _ "embed"

//go:embed controlcenter.html
var controlCenterHTML string
```

- [ ] **Step 3: Verify both builds compile**

Run: `go build -o /tmp/handymute ./cmd/handymute && GOOS=windows GOARCH=amd64 go build -o /tmp/hm.exe ./cmd/handymute`
Expected: PASS (both). If Linux build complains `jsc_value_to_string` undeclared, add `#include <jsc/jsc.h>` — but webkit2gtk-4.1 pulls JSC via `webkit2.h`, so this should not occur.

- [ ] **Step 4: Live run — tray + panel**

Run: `/tmp/handymute &` then look for the mic icon in the top-bar tray. Click it → menu → "Open HandyMute". 
Expected: the Control Center panel opens, showing the Enabled toggle, the two sliders, theme, and start-at-login — all reflecting `handymute.conf`. Toggle a slider; confirm `handymute.conf` updates (`cat ./handymute.conf` from the binary's dir, i.e. `/tmp`).

- [ ] **Step 5: Live run — glow + full integration**

With the app running and a call open, hold Ctrl+Space.
Expected: the tray icon swaps to the green glow variant and the panel's status dot lights (if open); teammates hear silence; output dims; release restores everything and the icon reverts.

- [ ] **Step 6: Commit**

```bash
git add cmd/handymute/ui_linux.go cmd/handymute/ui_windows.go cmd/handymute/controlcenter_html.go
git commit -m "feat(linux): GTK+WebKit Control Center and AppIndicator tray"
```

### Task 4.5: Start-at-login via XDG autostart

**Files:**
- Modify: `cmd/handymute/install_linux.go` (replace the stub)

- [ ] **Step 1: Replace `install_linux.go`**

```go
//go:build linux

package main

import (
	"os"
	"path/filepath"
)

// autostartPath returns ~/.config/autostart/handymute.desktop (honoring XDG_CONFIG_HOME).
func autostartPath() string {
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		home, _ := os.UserHomeDir()
		cfg = filepath.Join(home, ".config")
	}
	return filepath.Join(cfg, "autostart", "handymute.desktop")
}

func startupEnabled() bool {
	_, err := os.Stat(autostartPath())
	return err == nil
}

// installStartup writes an XDG autostart entry pointing at the current executable.
func installStartup() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	path := autostartPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	body := "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=HandyMute\n" +
		"Exec=" + exe + "\n" +
		"X-GNOME-Autostart-enabled=true\n" +
		"NoDisplay=true\n"
	return os.WriteFile(path, []byte(body), 0644)
}

func uninstallStartup() error {
	err := os.Remove(autostartPath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
```

- [ ] **Step 2: Verify both builds compile**

Run: `go build -o /tmp/handymute ./cmd/handymute && GOOS=windows GOARCH=amd64 go build -o /tmp/hm.exe ./cmd/handymute`
Expected: PASS.

- [ ] **Step 3: Live check the toggle**

Open the panel, flip "Start at login" on. Run: `cat ~/.config/autostart/handymute.desktop`
Expected: the desktop entry exists with `Exec=` pointing at the binary. Flip it off; confirm the file is removed.

- [ ] **Step 4: Commit**

```bash
git add cmd/handymute/install_linux.go
git commit -m "feat(linux): start-at-login via XDG autostart"
```

---

## Milestone 5 — Build script & docs

### Task 5.1: `build_linux.sh`

**Files:**
- Create: `build_linux.sh`

- [ ] **Step 1: Create `build_linux.sh`**

```bash
#!/usr/bin/env bash
# Build handymute for Linux (amd64). Run from the repo root.
# Requires: Go 1.25+, and dev headers for X11/Xtst, GTK3, WebKit2GTK-4.1, ayatana-appindicator3.
#   sudo apt-get install -y libx11-dev libxtst-dev libgtk-3-dev \
#       libwebkit2gtk-4.1-dev libayatana-appindicator3-dev
set -euo pipefail

if [ -x "$HOME/.goroot/bin/go" ]; then
    export GOROOT="$HOME/.goroot"
    export PATH="$HOME/.goroot/bin:$PATH"
fi

mkdir -p dist
echo "Building dist/handymute (linux/amd64)..."
CGO_ENABLED=1 go build -trimpath -o dist/handymute ./cmd/handymute
echo "Done:"
ls -la dist/handymute
```

- [ ] **Step 2: Make it executable and run it**

Run: `chmod +x build_linux.sh && ./build_linux.sh`
Expected: produces `dist/handymute`.

- [ ] **Step 3: Commit**

```bash
git add build_linux.sh
git commit -m "build: add build_linux.sh"
```

### Task 5.2: README Linux section

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update the status line and Roadmap**

In `README.md`, change the status note (line ~14) from Windows-only to note Linux (X11) support, and under "Roadmap" move the PipeWire item to a new "## Linux" section describing the zero-config approach. Add this section after "## How it works":

```markdown
## Linux (X11)

HandyMute runs natively on Linux. Build it:

\`\`\`bash
sudo apt-get install -y libx11-dev libxtst-dev libgtk-3-dev \
    libwebkit2gtk-4.1-dev libayatana-appindicator3-dev
./build_linux.sh   # -> dist/handymute
\`\`\`

No virtual cable and **no audio setup** is needed. While Ctrl+Space is held, HandyMute lowers
the capture-stream volume of every app reading your mic *except* Handy (so teammates hear your
chosen level, default silent) and dims your default output. It speaks the PulseAudio protocol,
so it works on both PipeWire and PulseAudio systems. The Control Center and tray behave as on
Windows. Phase 1 targets X11; Wayland global-hotkey support is planned.
\`\`\`
```

(Replace the escaped backticks with real ones when editing.)

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: README Linux (X11) build + usage section"
```

---

## Final verification

- [ ] **Linux build + tests pass:** `go build ./cmd/handymute && go test ./cmd/handymute/`
- [ ] **Windows regression build passes:** `GOOS=windows GOARCH=amd64 go build -o /tmp/hm.exe ./cmd/handymute`
- [ ] **End-to-end on this machine:** with Handy + a Teams/Discord call, holding Ctrl+Space silences teammates and dims output; release restores; tray glows while held; the panel reads/writes settings; start-at-login toggle writes/removes the autostart entry.
- [ ] **Push the branch and open a PR upstream** (only when the user asks).

---

## Notes for the implementer

- **Restore safety:** the worker saves prior volumes per source-output index and restores them on release. If the app is killed mid-hold, volumes stay ducked — this matches the Windows behavior and is acceptable; a fresh hold+release re-normalizes.
- **Handy must be excluded, never targeted.** The whole correctness of the audio layer rests on `isHandy`. If Handy's process/app/node identifiers ever change, update `isHandy` and its test.
- **v1 limitation:** any non-Handy capturer (OBS, another recorder) is also silenced during a hold. Documented; a future exclusion list can refine this.
- **GTK main loop owns the main thread.** All GTK/WebKit/AppIndicator calls from Go go through `g_idle_add` (see `ui_eval`, `ui_show`, `ui_set_icon`) so they execute on the GTK thread. Do not call GTK functions directly from other goroutines.
