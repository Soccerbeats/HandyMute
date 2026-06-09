//go:build linux

package main

import (
	"encoding/json"
	"fmt"
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
