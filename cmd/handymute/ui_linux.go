//go:build linux

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// runUI hosts the tray + Control Center. Real implementation lands in Milestone 4.
func runUI(settings *Settings, cmd chan<- bool, status <-chan bool) error {
	logf("handymute (linux) running — UI not yet implemented; press Ctrl+C to quit")
	go func() {
		for range status { // drain status so the glow goroutine never blocks
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	return nil
}
