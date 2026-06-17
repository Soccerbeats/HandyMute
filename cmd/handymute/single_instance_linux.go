//go:build linux

package main

import (
	"os"
	"path/filepath"
	"syscall"
)

// instanceLock holds the exclusive lock file for this process's lifetime.
var instanceLock *os.File

// claimSingleInstance returns true if this is the first HandyMute instance, using an exclusive
// flock on a lock file. A second instance fails the non-blocking lock and returns false (the
// caller should exit). Raising the existing window from a second instance is not yet wired on
// Linux — see waitForActivate, which is a no-op here.
func claimSingleInstance() bool {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	f, err := os.OpenFile(filepath.Join(dir, "handymute.lock"), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return true // can't create the lock file; don't block startup
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return false
	}
	instanceLock = f // hold the lock for the process lifetime
	return true
}
