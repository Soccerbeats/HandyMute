//go:build linux

package main

import (
	"os"
	"path/filepath"
)

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
