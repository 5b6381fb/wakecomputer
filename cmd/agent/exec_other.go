//go:build !linux && !windows

package main

import "fmt"

func runShutdown(action string) error {
	return fmt.Errorf("power control is not implemented on this platform")
}
