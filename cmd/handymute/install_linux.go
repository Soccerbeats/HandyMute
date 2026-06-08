//go:build linux

package main

func startupEnabled() bool    { return false }
func installStartup() error   { return nil }
func uninstallStartup() error { return nil }
