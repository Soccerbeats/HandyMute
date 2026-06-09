//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

const runKeyName = "HandyMute"
const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

// startupEnabled reports whether the auto-start Run key is currently present and points at
// the running executable.
func startupEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(runKeyName)
	return err == nil
}

// installStartup registers the current executable under the per-user Run key so it launches
// at login. No admin rights required.
func installStartup() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(runKeyName, `"`+exe+`"`)
}

// uninstallStartup removes the auto-start registration.
func uninstallStartup() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	err = k.DeleteValue(runKeyName)
	if err == registry.ErrNotExist {
		return nil
	}
	return err
}
