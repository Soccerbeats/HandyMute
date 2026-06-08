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
	speakerDuck float32 // 0..1 — level all non-cable outputs drop to while dictating
	teamsLevel  float32 // 0..1 — level the cable feed (what teammates hear) drops to while dictating
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
		teamsLevel:  0.00,
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

func (s *Settings) SetTeamsLevel(v float32) {
	s.mu.Lock()
	s.teamsLevel = clamp01(v)
	s.mu.Unlock()
	s.save()
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
// malformed. Recognized keys: enabled, speaker_duck, teams_level, cable.
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
		case "teams_level":
			if v, err := strconv.ParseFloat(val, 32); err == nil {
				s.teamsLevel = clamp01(float32(v))
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
	return s
}

// save writes the current settings to disk. Best-effort: a failure is logged, not fatal.
func (s *Settings) save() {
	s.mu.RLock()
	body := fmt.Sprintf(
		"# HandyMute settings — edit via the tray UI (or here, one key=value per line)\n"+
			"enabled=%t\n"+
			"speaker_duck=%.3f\n"+
			"teams_level=%.3f\n"+
			"cable=%s\n"+
			"theme=%s\n",
		s.enabled, s.speakerDuck, s.teamsLevel, s.cableMatch, s.theme)
	s.mu.RUnlock()

	if err := os.WriteFile(settingsPath(), []byte(body), 0644); err != nil {
		logf("could not save settings: %v", err)
	}
}
