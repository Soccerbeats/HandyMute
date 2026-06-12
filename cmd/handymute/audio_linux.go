//go:build linux

package main

import (
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
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

func setSinkInputVolume(index, percent int) {
	_ = exec.Command("pactl", "set-sink-input-volume", strconv.Itoa(index), strconv.Itoa(percent)+"%").Run()
}

// meetingAppHints identify a playback stream owned by a meeting/voice app, matched against
// the sink-input's application.process.binary / application.name (case-insensitive).
var meetingAppHints = []string{"teams", "discord", "teamspeak", "ts3client", "zoom"}

// applyMeetingVolume sets the raw output volume of every running meeting app's playback
// stream to level (0..1) — independent of the ctrl+space ducking. PipeWire/Pulse apply
// per-stream volume in software, so this attenuates only those apps.
func applyMeetingVolume(level float32) {
	out, err := pactlJSON("list", "sink-inputs")
	if err != nil {
		logf("meeting volume: list sink-inputs: %v", err)
		return
	}
	var inputs []struct {
		Index      int               `json:"index"`
		Properties map[string]string `json:"properties"`
	}
	if err := json.Unmarshal(out, &inputs); err != nil {
		logf("meeting volume: parse sink-inputs: %v", err)
		return
	}
	pct := pctScalar(level)
	set := 0
	for _, in := range inputs {
		if isMeetingSinkInput(in.Properties) {
			setSinkInputVolume(in.Index, pct)
			set++
		}
	}
	logf("meeting volume -> %d%% on %d app stream(s)", pct, set)
}

func isMeetingSinkInput(props map[string]string) bool {
	hay := strings.ToLower(props["application.process.binary"] + " " + props["application.name"])
	for _, h := range meetingAppHints {
		if strings.Contains(hay, h) {
			return true
		}
	}
	return false
}

func trimNewline(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

// pctScalar converts a 0..1 scalar to a rounded 0..100 percent.
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
