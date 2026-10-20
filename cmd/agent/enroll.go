package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/wakecomputer/wakecomputer/internal/version"
)

type enrollResponse struct {
	HostID string `json:"hostId"`
	Token  string `json:"token"`
}

func enroll(serverURL, code, configPath string, insecure bool) error {
	if serverURL == "" || code == "" {
		return fmt.Errorf("--server and --code are required")
	}
	certPEM, keyPEM, fingerprint, err := generateCert()
	if err != nil {
		return err
	}
	hostname, _ := os.Hostname()
	payload := map[string]any{
		"code":        code,
		"hostname":    hostname,
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
		"version":     version.Version,
		"certPem":     string(certPEM),
		"fingerprint": fingerprint,
		"ips":         reportIPs(serverURL),
	}
	body, _ := json.Marshal(payload)
	client := httpClient(insecure, 15*time.Second)
	resp, err := client.Post(trimSlash(serverURL)+"/agent/v1/enroll", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("connect to server: %w", err)
	}
	defer resp.Body.Close()
	var out enrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("server returned %d and an invalid body", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("enrollment rejected (http %d)", resp.StatusCode)
	}

	dir := configDir()
	if configPath != "" {
		dir = filepath.Dir(configPath)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	certFile := filepath.Join(dir, "agent.crt")
	keyFile := filepath.Join(dir, "agent.key")
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		return err
	}
	keyRaw, err := x509.MarshalECPrivateKey(keyPEM)
	if err != nil {
		return err
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyRaw}), 0o600); err != nil {
		return err
	}
	cfg := &Config{
		ServerURL:   trimSlash(serverURL),
		HostID:      out.HostID,
		Token:       out.Token,
		Listen:      ":45871",
		CertFile:    certFile,
		KeyFile:     keyFile,
		Fingerprint: fingerprint,
		Insecure:    insecure,
	}
	if err := saveConfig(configPath, cfg); err != nil {
		return err
	}
	fmt.Printf("enrolled successfully: hostId=%s\nconfig: %s\n", out.HostID, configPath)
	return nil
}

// generateCert creates a self-signed P-256 certificate and returns
// (certPEM, privateKey, sha256 fingerprint).
func generateCert() ([]byte, *ecdsa.PrivateKey, string, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, "", err
	}
	hostname, _ := os.Hostname()
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: hostname, Organization: []string{"WakeComputer Agent"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{hostname},
	}
	for _, ip := range localIPv4() {
		tpl.IPAddresses = append(tpl.IPAddresses, net.ParseIP(ip))
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, "", err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, "", err
	}
	sum := sha256.Sum256(cert.Raw)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), key, hex.EncodeToString(sum[:]), nil
}

func localIPv4() []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []string
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if v4 := ipnet.IP.To4(); v4 != nil {
				out = append(out, v4.String())
			}
		}
	}
	return out
}

// reportIPs puts the interface used to reach the server first. On multi-NIC
// hosts (Docker, VPN, WSL) this is far more reliable than enumerating
// interfaces, because it is the address the server can actually reach.
func reportIPs(serverURL string) []string {
	var out []string
	if ip := outboundIP(serverURL); ip != "" {
		out = append(out, ip)
	}
	for _, ip := range localIPv4() {
		if len(out) == 0 || out[0] != ip {
			out = append(out, ip)
		}
	}
	return out
}

func outboundIP(serverURL string) string {
	u, err := url.Parse(serverURL)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	conn, err := net.DialTimeout("udp", net.JoinHostPort(u.Hostname(), port), 2*time.Second)
	if err != nil {
		return ""
	}
	defer conn.Close()
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return addr.IP.String()
	}
	return ""
}

func httpClient(insecure bool, timeout time.Duration) *http.Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: insecure,
		},
	}
	return &http.Client{Timeout: timeout, Transport: tr}
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
