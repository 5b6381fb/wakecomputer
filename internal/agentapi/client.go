package agentapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/wakecomputer/wakecomputer/internal/secretbox"
	"github.com/wakecomputer/wakecomputer/internal/store"
)

type Command struct {
	ID           string `json:"id"`
	Action       string `json:"action"`
	DelaySeconds int    `json:"delaySeconds"`
	Reason       string `json:"reason,omitempty"`
}

type Client struct {
	tokenKey      []byte
	allowInsecure bool
	log           *slog.Logger
}

func New(tokenKey []byte, allowInsecure bool, log *slog.Logger) *Client {
	return &Client{tokenKey: tokenKey, allowInsecure: allowInsecure, log: log}
}

// SendCommand calls the agent directly over the LAN.
func (c *Client) SendCommand(ctx context.Context, h *store.Host, cmd Command) error {
	if h.AgentURL == "" {
		return fmt.Errorf("host has no agent")
	}
	token, err := secretbox.Open(c.tokenKey, h.AgentTokenEnc)
	if err != nil {
		return fmt.Errorf("decrypt agent token: %w", err)
	}
	body, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	fingerprint := normalizeFingerprint(h.AgentFingerprint)
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if fingerprint == "" && c.allowInsecure {
		tlsCfg.InsecureSkipVerify = true
	} else {
		tlsCfg.InsecureSkipVerify = true // verification is done by the pinned fingerprint below
		tlsCfg.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("agent presented no certificate")
			}
			sum := sha256.Sum256(rawCerts[0])
			got := hex.EncodeToString(sum[:])
			if got != fingerprint {
				return fmt.Errorf("agent certificate fingerprint mismatch")
			}
			return nil
		}
	}

	reqCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		strings.TrimRight(h.AgentURL, "/")+"/v1/command", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	httpClient := &http.Client{
		Timeout: 6 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig:   tlsCfg,
			DisableKeepAlives: true,
		},
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("agent unreachable: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("agent returned %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	return nil
}

func normalizeFingerprint(fp string) string {
	fp = strings.ToLower(strings.TrimSpace(fp))
	fp = strings.TrimPrefix(fp, "sha256:")
	fp = strings.ReplaceAll(fp, ":", "")
	return fp
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
