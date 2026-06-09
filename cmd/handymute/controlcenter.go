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
	case "outbound_preset":
		var name string
		json.Unmarshal(m.Value, &name)
		settings.SetOutboundPreset(name)
		pushControlState(settings, eval)
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
		"enabled":        settings.Enabled(),
		"teams":          pct(settings.TeamsLevel()),
		"outboundPreset": settings.OutboundPreset(),
		"speaker":        pct(settings.SpeakerDuck()),
		"startup":        startupEnabled(),
		"theme":          settings.Theme(),
	}
	b, err := json.Marshal(state)
	if err != nil {
		return
	}
	eval("applyState(" + string(b) + ")")
}

// pct converts a 0..1 scalar to a 0..100 integer for the sliders/labels.
func pct(v float32) int {
	return int(v*100 + 0.5)
}
