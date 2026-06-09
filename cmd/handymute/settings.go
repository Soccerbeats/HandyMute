package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Settings is the live, mutable configuration shared between the keyboard hook, the audio
// worker, and the tray UI. All access goes through methods so reads and writes are
// goroutine-safe. Every setter persists to disk so changes survive a restart.
type Settings struct {
	mu sync.RWMutex

	enabled     bool    // master on/off; when off, ctrl+space is ignored (Handy still gets it)
	speakerDuck float32 // 0..1 — Inbound: level all non-cable outputs drop to while dictating
	teamsLevel  float32 // 0..1 — live Outbound level (cable feed) applied while dictating
	outMute     float32 // remembered Outbound level for the Mute preset
	outQuiet    float32 // remembered Outbound level for the Quiet preset
	outFull     float32 // remembered Outbound level for the Full preset
	outPreset   string  // active Outbound preset: "mute" | "quiet" | "full"
	cableMatch  string  // case-insensitive substring identifying the VB-Cable render endpoint
	theme       string  // UI theme for the control-center panel: "dark" or "light"
}

// Snapshot is an immutable copy of the settings the audio worker needs for one toggle.
type Snapshot struct {
	SpeakerDuck float32
	TeamsLevel  float32
	CableMatch  string
}

func defaultSettings() *Settings {
	return &Settings{
		enabled:     true,
		speakerDuck: 0.20,
		teamsLevel:  0.06,
		outMute:     0.00,
		outQuiet:    0.06,
		outFull:     1.00,
		outPreset:   "quiet",
		cableMatch:  "cable input",
		theme:       "dark",
	}
}

func (s *Settings) Enabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled
}

func (s *Settings) SetEnabled(v bool) {
	s.mu.Lock()
	s.enabled = v
	s.mu.Unlock()
	s.save()
}

func (s *Settings) SpeakerDuck() float32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.speakerDuck
}

func (s *Settings) SetSpeakerDuck(v float32) {
	s.mu.Lock()
	s.speakerDuck = clamp01(v)
	s.mu.Unlock()
	s.save()
}

func (s *Settings) Theme() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.theme
}

func (s *Settings) SetTheme(v string) {
	if v != "light" {
		v = "dark"
	}
	s.mu.Lock()
	s.theme = v
	s.mu.Unlock()
	s.save()
}

func (s *Settings) TeamsLevel() float32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.teamsLevel
}

// SetTeamsLevel stores v as the live Outbound level and remembers it for the active preset,
// so reselecting that preset later restores this level.
func (s *Settings) SetTeamsLevel(v float32) {
	v = clamp01(v)
	s.mu.Lock()
	switch s.outPreset {
	case "mute":
		s.outMute = v
	case "full":
		s.outFull = v
	default:
		s.outQuiet = v
	}
	s.teamsLevel = v
	s.mu.Unlock()
	s.save()
}

func (s *Settings) OutboundPreset() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.outPreset
}

// SetOutboundPreset activates a preset and restores the Outbound level remembered for it.
func (s *Settings) SetOutboundPreset(name string) {
	if name != "mute" && name != "full" {
		name = "quiet"
	}
	s.mu.Lock()
	s.outPreset = name
	s.teamsLevel = s.resolveOutLocked()
	s.mu.Unlock()
	s.save()
}

// resolveOutLocked returns the level remembered for the active preset. Caller holds the lock.
func (s *Settings) resolveOutLocked() float32 {
	switch s.outPreset {
	case "mute":
		return s.outMute
	case "full":
		return s.outFull
	default:
		return s.outQuiet
	}
}

// Snapshot returns a consistent copy of the values the worker applies on a toggle.
func (s *Settings) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Snapshot{
		SpeakerDuck: s.speakerDuck,
		TeamsLevel:  s.teamsLevel,
		CableMatch:  s.cableMatch,
	}
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// settingsPath is the on-disk location of the persisted settings, next to the executable.
func settingsPath() string {
	return filepath.Join(exeDir(), "handymute.conf")
}

// loadSettings reads persisted settings, falling back to defaults for anything missing or
// malformed. Recognized keys: enabled, speaker_duck, outbound_mute/quiet/full, outbound_preset, cable.
func loadSettings() *Settings {
	s := defaultSettings()

	f, err := os.Open(settingsPath())
	if err != nil {
		return s
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(strings.ToLower(key))
		val = strings.TrimSpace(val)
		switch key {
		case "enabled":
			s.enabled = val == "true" || val == "1" || val == "yes"
		case "speaker_duck":
			if v, err := strconv.ParseFloat(val, 32); err == nil {
				s.speakerDuck = clamp01(float32(v))
			}
		case "outbound_mute":
			if v, err := strconv.ParseFloat(val, 32); err == nil {
				s.outMute = clamp01(float32(v))
			}
		case "outbound_quiet":
			if v, err := strconv.ParseFloat(val, 32); err == nil {
				s.outQuiet = clamp01(float32(v))
			}
		case "outbound_full":
			if v, err := strconv.ParseFloat(val, 32); err == nil {
				s.outFull = clamp01(float32(v))
			}
		case "outbound_preset":
			if val == "mute" || val == "quiet" || val == "full" {
				s.outPreset = val
			}
		case "cable":
			if val != "" {
				s.cableMatch = strings.ToLower(val)
			}
		case "theme":
			if val == "light" || val == "dark" {
				s.theme = val
			}
		}
	}
	s.teamsLevel = s.resolveOutLocked() // live Outbound level always follows the active preset
	return s
}

// save writes the current settings to disk. Best-effort: a failure is logged, not fatal.
func (s *Settings) save() {
	s.mu.RLock()
	body := fmt.Sprintf(
		"# HandyMute settings — edit via the tray UI (or here, one key=value per line)\n"+
			"enabled=%t\n"+
			"speaker_duck=%.3f\n"+
			"outbound_mute=%.3f\n"+
			"outbound_quiet=%.3f\n"+
			"outbound_full=%.3f\n"+
			"outbound_preset=%s\n"+
			"cable=%s\n"+
			"theme=%s\n",
		s.enabled, s.speakerDuck, s.outMute, s.outQuiet, s.outFull, s.outPreset, s.cableMatch, s.theme)
	s.mu.RUnlock()

	if err := os.WriteFile(settingsPath(), []byte(body), 0644); err != nil {
		logf("could not save settings: %v", err)
	}
}
