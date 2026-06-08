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
