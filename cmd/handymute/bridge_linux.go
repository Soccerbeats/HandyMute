//go:build linux

package main

import "errors"

// Linux uses no virtual audio cable, so the bridge flags are not supported.
func setupBridge() error  { return errors.New("-setup-bridge is Windows-only; not needed on Linux") }
func removeBridge() error { return errors.New("-remove-bridge is Windows-only; not needed on Linux") }
