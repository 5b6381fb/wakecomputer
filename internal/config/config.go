package config

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr        string
	DataDir         string
	PublicURL       string
	TrustedProxies  []string
	JWTSecret       string
	AccessTTL       time.Duration
	RefreshTTL      time.Duration
	LogLevel        string
	LogFormat       string
	WolBroadcast    string
	WolPort         int
	LANSubnet       string
	AdminUser       string
	AdminPassword   string
	MaxOpenConns    int
	ShutdownTimeout time.Duration
}

func Load() (*Config, error) {
	c := &Config{
		HTTPAddr:        env("WC_HTTP_ADDR", "0.0.0.0:8085"),
		DataDir:         env("WC_DATA_DIR", "/data"),
		PublicURL:       strings.TrimRight(env("WC_PUBLIC_URL", ""), "/"),
		TrustedProxies:  splitCSV(env("WC_TRUSTED_PROXIES", "")),
		LogLevel:        env("WC_LOG_LEVEL", "info"),
		LogFormat:       env("WC_LOG_FORMAT", "text"),
		WolBroadcast:    env("WC_WOL_DEFAULT_BROADCAST", "255.255.255.255"),
		LANSubnet:       env("WC_LAN_SUBNET", ""),
		AdminUser:       env("WC_ADMIN_USER", "admin"),
		AdminPassword:   env("WC_ADMIN_PASSWORD", ""),
		MaxOpenConns:    envInt("WC_MAX_OPEN_CONNS", 4),
		ShutdownTimeout: 10 * time.Second,
	}
	var err error
	if c.AccessTTL, err = time.ParseDuration(env("WC_ACCESS_TTL", "15m")); err != nil {
		return nil, fmt.Errorf("WC_ACCESS_TTL: %w", err)
	}
	if c.RefreshTTL, err = time.ParseDuration(env("WC_REFRESH_TTL", "720h")); err != nil {
		return nil, fmt.Errorf("WC_REFRESH_TTL: %w", err)
	}
	if c.WolPort, err = strconv.Atoi(env("WC_WOL_PORT", "9")); err != nil {
		return nil, fmt.Errorf("WC_WOL_PORT: %w", err)
	}
	c.JWTSecret = env("WC_JWT_SECRET", "")
	if err := os.MkdirAll(c.DataDir, 0o750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	if c.JWTSecret == "" {
		secret, err := loadOrCreateSecret(filepath.Join(c.DataDir, "secret"))
		if err != nil {
			return nil, err
		}
		c.JWTSecret = secret
	}
	return c, nil
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	v, err := strconv.Atoi(env(k, ""))
	if err != nil || v <= 0 {
		return def
	}
	return v
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func loadOrCreateSecret(path string) (string, error) {
	if b, err := os.ReadFile(path); err == nil && len(b) >= 32 {
		return strings.TrimSpace(string(b)), nil
	}
	b := make([]byte, 48)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return "", fmt.Errorf("write secret: %w", err)
	}
	return string(b), nil
}
