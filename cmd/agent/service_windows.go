//go:build windows

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const serviceName = "WakeAgent"

type serviceHandler struct {
	configPath string
}

func (h *serviceHandler) Execute(_ []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}
	cfg, err := loadConfig(h.configPath)
	if err != nil {
		log.Printf("load config: %v", err)
		return false, 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		if err := runAgent(ctx, cfg); err != nil {
			log.Printf("agent stopped: %v", err)
		}
	}()
	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			cancel()
			return false, 0
		}
	}
	return false, 0
}

func runService(configPath string) error {
	if configPath == "" {
		configPath = defaultConfigPath()
	}
	abs, _ := filepath.Abs(configPath)
	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if !isService {
		cfg, err := loadConfig(abs)
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return runAgent(ctx, cfg)
	}
	return svc.Run(serviceName, &serviceHandler{configPath: abs})
}

func installService(configPath string) error {
	if configPath == "" {
		configPath = defaultConfigPath()
	}
	abs, _ := filepath.Abs(configPath)
	if _, err := loadConfig(abs); err != nil {
		return fmt.Errorf("enroll first: %w", err)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	// Replace an existing installation.
	if s, err := m.OpenService(serviceName); err == nil {
		_ = s.Delete()
		_ = s.Close()
	}
	s, err := m.CreateService(serviceName, exe, mgr.Config{
		DisplayName:  "WakeComputer Agent",
		Description:  "Remote shutdown/reboot and heartbeat agent for WakeComputer",
		StartType:    mgr.StartAutomatic,
		ErrorControl: mgr.ErrorNormal,
	}, "run", "-config", abs)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 15 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
	}, 24*60*60); err != nil {
		log.Printf("service recovery actions not set: %v", err)
	}
	if err := s.Start(); err != nil {
		log.Printf("service installed but failed to start: %v", err)
	}
	fmt.Println("service WakeAgent installed and started")
	return nil
}

func uninstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service not installed: %w", err)
	}
	defer s.Close()
	_, _ = s.Control(svc.Stop)
	if err := s.Delete(); err != nil {
		return err
	}
	fmt.Println("service WakeAgent removed")
	return nil
}
