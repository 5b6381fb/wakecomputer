//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func runShutdown(action string) error {
	var args []string
	switch action {
	case "shutdown":
		args = []string{"/s", "/f", "/t", "0", "/d", "p:0:0", "/c", "WakeComputer remote shutdown"}
	case "reboot":
		args = []string{"/r", "/f", "/t", "0", "/d", "p:0:0", "/c", "WakeComputer remote reboot"}
	case "cancel":
		args = []string{"/a"}
	default:
		return fmt.Errorf("unsupported action %q", action)
	}
	out, err := exec.Command("shutdown.exe", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("shutdown.exe: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
