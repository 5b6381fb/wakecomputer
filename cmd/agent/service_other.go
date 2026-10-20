//go:build !windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func runService(configPath string) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runAgent(ctx, cfg)
}

func installService(configPath string) error {
	fmt.Println("This platform uses systemd. The install.sh script installs the unit automatically.")
	fmt.Println("Manual setup:")
	fmt.Println("  sudo install -m 0755 wake-agent /usr/local/bin/wake-agent")
	fmt.Println("  sudo wake-agent enroll --server <url> --code <code>")
	fmt.Println("  sudo systemctl enable --now wake-agent")
	return nil
}

func uninstallService() error {
	fmt.Println("Run: sudo systemctl disable --now wake-agent && sudo rm -f /etc/systemd/system/wake-agent.service /usr/local/bin/wake-agent")
	return nil
}
