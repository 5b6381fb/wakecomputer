//go:build linux

package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func runShutdown(action string) error {
	switch action {
	case "cancel":
		out, err := exec.Command("shutdown", "-c").CombinedOutput()
		if err != nil {
			return fmt.Errorf("shutdown -c: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	case "shutdown":
		if out, err := exec.Command("systemctl", "poweroff").CombinedOutput(); err == nil {
			return nil
		} else if out2, err2 := exec.Command("shutdown", "-h", "now").CombinedOutput(); err2 == nil {
			return nil
		} else {
			return fmt.Errorf("poweroff failed: %v %s / %v %s", err, strings.TrimSpace(string(out)), err2, strings.TrimSpace(string(out2)))
		}
	case "reboot":
		if out, err := exec.Command("systemctl", "reboot").CombinedOutput(); err == nil {
			return nil
		} else if out2, err2 := exec.Command("shutdown", "-r", "now").CombinedOutput(); err2 == nil {
			return nil
		} else {
			return fmt.Errorf("reboot failed: %v %s / %v %s", err, strings.TrimSpace(string(out)), err2, strings.TrimSpace(string(out2)))
		}
	}
	return fmt.Errorf("unsupported action %q", action)
}
