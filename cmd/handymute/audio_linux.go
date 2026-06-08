//go:build linux

package main

// muteWorker applies/reverses the duck on each hold transition. Real implementation lands in
// Milestone 2.
func muteWorker(cmd <-chan bool, settings *Settings) {
	for range cmd {
	}
}
