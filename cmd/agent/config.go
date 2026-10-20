package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type Config struct {
	ServerURL   string `json:"serverUrl"`
	HostID      string `json:"hostId"`
	Token       string `json:"token"`
	Listen      string `json:"listen"`
	CertFile    string `json:"certFile"`
	KeyFile     string `json:"keyFile"`
	Fingerprint string `json:"fingerprint"`
	Insecure    bool   `json:"insecure"`
}

func configDir() string {
	if runtime.GOOS == "windows" {
		if pd := os.Getenv("ProgramData"); pd != "" {
			return filepath.Join(pd, "WakeComputer")
		}
		return `C:\ProgramData\WakeComputer`
	}
	return "/etc/wake-agent"
}

func defaultConfigPath() string {
	return filepath.Join(configDir(), "config.json")
}

func certPaths() (string, string) {
	dir := configDir()
	return filepath.Join(dir, "agent.crt"), filepath.Join(dir, "agent.key")
}

func loadConfig(path string) (*Config, error) {
	if path == "" {
		path = defaultConfigPath()
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if c.ServerURL == "" || c.HostID == "" || c.Token == "" {
		return nil, fmt.Errorf("config is incomplete, run 'wake-agent enroll' first")
	}
	if c.Listen == "" {
		c.Listen = ":45871"
	}
	dir := filepath.Dir(path)
	if c.CertFile == "" {
		c.CertFile = filepath.Join(dir, "agent.crt")
	}
	if c.KeyFile == "" {
		c.KeyFile = filepath.Join(dir, "agent.key")
	}
	return &c, nil
}

func saveConfig(path string, c *Config) error {
	if path == "" {
		path = defaultConfigPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return err
	}
	return nil
}
