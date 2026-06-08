//go:build linux

package main

// runHook watches Ctrl+Space. Real implementation (X RECORD) lands in Milestone 3.
func runHook(cmd chan<- bool, status chan<- bool, settings *Settings) error {
	select {} // block forever
}
