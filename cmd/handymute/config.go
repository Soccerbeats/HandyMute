package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// exeDir returns the directory containing the running executable.
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// logf prints a timestamped line to stdout and appends it to handymute.log next to the
// executable, so the app remains debuggable when run without a console (autostart).
func logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	stamp := time.Now().Format("15:04:05")
	line := stamp + " " + msg
	fmt.Println(line)
	if f, err := os.OpenFile(filepath.Join(exeDir(), "handymute.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		fmt.Fprintln(f, line)
		f.Close()
	}
}

// send delivers a mute/unmute command without ever blocking the caller.
func send(cmd chan<- bool, v bool) {
	select {
	case cmd <- v:
	default:
	}
}
