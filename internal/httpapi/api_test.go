package httpapi

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wakecomputer/wakecomputer/internal/agentapi"
	"github.com/wakecomputer/wakecomputer/internal/auth"
	"github.com/wakecomputer/wakecomputer/internal/config"
	"github.com/wakecomputer/wakecomputer/internal/job"
	"github.com/wakecomputer/wakecomputer/internal/secretbox"
	"github.com/wakecomputer/wakecomputer/internal/sse"
	"github.com/wakecomputer/wakecomputer/internal/status"
	"github.com/wakecomputer/wakecomputer/internal/store"
)

type testEnv struct {
	srv   *httptest.Server
	st    *store.Store
	token string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		HTTPAddr:        "127.0.0.1:0",
		DataDir:         dir,
		JWTSecret:       "0123456789abcdef0123456789abcdef0123456789abcdef",
		AccessTTL:       15 * time.Minute,
		RefreshTTL:      720 * time.Hour,
		WolBroadcast:    "192.168.1.255",
		WolPort:         9,
		LogLevel:        "error",
		LogFormat:       "text",
		MaxOpenConns:    4,
		ShutdownTimeout: time.Second,
	}
	st, err := store.Open(filepath.Join(dir, "wake.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser("admin", hash); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := sse.NewHub()
	tokenKey := secretbox.Key(cfg.JWTSecret, "agent-token")
	statusSvc := status.New(st, hub, logger)
	agentClient := agentapi.New(tokenKey, true, logger)
	jobs := job.New(st, hub, agentClient, statusSvc, logger, cfg.WolBroadcast, cfg.WolPort, 2)

	api := New(Options{
		Config: cfg, Store: st, Hub: hub, Status: statusSvc,
		Jobs: jobs, Agent: agentClient, Logger: logger, TokenKey: tokenKey,
	})
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)

	env := &testEnv{srv: srv, st: st}
	env.login(t)
	return env
}

func (e *testEnv) do(t *testing.T, method, path, token string, body any) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func (e *testEnv) login(t *testing.T) {
	t.Helper()
	code, raw := e.do(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": "admin", "password": "password123",
	})
	if code != http.StatusOK {
		t.Fatalf("login = %d: %s", code, raw)
	}
	var out tokenResp
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	e.token = out.AccessToken
}

func TestHealthAndAuth(t *testing.T) {
	env := newTestEnv(t)

	code, _ := env.do(t, http.MethodGet, "/healthz", "", nil)
	if code != http.StatusOK {
		t.Fatalf("healthz = %d", code)
	}
	code, _ = env.do(t, http.MethodGet, "/api/v1/hosts", "", nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("unauthorized list = %d", code)
	}
	code, _ = env.do(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": "admin", "password": "wrong",
	})
	if code != http.StatusUnauthorized {
		t.Fatalf("wrong password = %d", code)
	}
}

func TestHostCRUDAndAudit(t *testing.T) {
	env := newTestEnv(t)

	code, raw := env.do(t, http.MethodPost, "/api/v1/hosts", env.token, map[string]any{
		"name": "书房主机", "osType": "windows",
		"mac": "AA:BB:CC:DD:EE:FF", "ip": "192.168.1.20",
		"broadcastTargets": []string{"192.168.1.255:9"}, "probePort": 3389,
	})
	if code != http.StatusCreated {
		t.Fatalf("create = %d: %s", code, raw)
	}
	var host store.Host
	if err := json.Unmarshal(raw, &host); err != nil {
		t.Fatal(err)
	}
	if !host.Capability.Wake || host.Capability.Shutdown {
		t.Fatalf("capabilities = %+v", host.Capability)
	}

	code, _ = env.do(t, http.MethodPost, "/api/v1/hosts", env.token, map[string]any{
		"name": "书房主机", "mac": "AA:BB:CC:DD:EE:FF",
	})
	if code != http.StatusConflict {
		t.Fatalf("duplicate = %d", code)
	}

	code, raw = env.do(t, http.MethodPatch, "/api/v1/hosts/"+host.ID, env.token, map[string]any{
		"name": "书房主机-改",
	})
	if code != http.StatusOK {
		t.Fatalf("patch = %d: %s", code, raw)
	}

	code, raw = env.do(t, http.MethodGet, "/api/v1/hosts", env.token, nil)
	if code != http.StatusOK {
		t.Fatalf("list = %d", code)
	}
	var list struct {
		Items []store.Host `json:"items"`
	}
	_ = json.Unmarshal(raw, &list)
	if len(list.Items) != 1 || list.Items[0].Name != "书房主机-改" {
		t.Fatalf("list = %+v", list.Items)
	}

	code, raw = env.do(t, http.MethodGet, "/api/v1/audit?limit=50", env.token, nil)
	if code != http.StatusOK {
		t.Fatalf("audit = %d", code)
	}
	if !bytes.Contains(raw, []byte("host.create")) {
		t.Fatalf("audit missing host.create: %s", raw)
	}

	code, _ = env.do(t, http.MethodPost, "/api/v1/hosts/"+host.ID+"/power", env.token, map[string]any{
		"action": "shutdown",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("power without agent = %d", code)
	}

	code, _ = env.do(t, http.MethodDelete, "/api/v1/hosts/"+host.ID, env.token, nil)
	if code != http.StatusNoContent {
		t.Fatalf("delete = %d", code)
	}
}

func TestRefreshRotation(t *testing.T) {
	env := newTestEnv(t)
	code, raw := env.do(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": "admin", "password": "password123",
	})
	if code != http.StatusOK {
		t.Fatalf("login = %d", code)
	}
	var first tokenResp
	_ = json.Unmarshal(raw, &first)

	code, raw = env.do(t, http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{
		"refreshToken": first.RefreshToken,
	})
	if code != http.StatusOK {
		t.Fatalf("refresh = %d: %s", code, raw)
	}
	var second tokenResp
	_ = json.Unmarshal(raw, &second)
	if second.RefreshToken == first.RefreshToken || second.AccessToken == "" {
		t.Fatal("tokens were not rotated")
	}

	code, _ = env.do(t, http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{
		"refreshToken": first.RefreshToken,
	})
	if code != http.StatusUnauthorized {
		t.Fatalf("reused refresh token = %d", code)
	}
}

func TestAgentEnrollmentAndHeartbeat(t *testing.T) {
	env := newTestEnv(t)
	code, raw := env.do(t, http.MethodPost, "/api/v1/hosts", env.token, map[string]any{
		"name": "linux-box", "osType": "linux", "mac": "AA:BB:CC:DD:EE:01",
	})
	if code != http.StatusCreated {
		t.Fatalf("create = %d: %s", code, raw)
	}
	var host store.Host
	_ = json.Unmarshal(raw, &host)

	code, raw = env.do(t, http.MethodPost, "/api/v1/hosts/"+host.ID+"/agent/enroll-code", env.token, nil)
	if code != http.StatusOK {
		t.Fatalf("enroll-code = %d: %s", code, raw)
	}
	var ec struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(raw, &ec)
	if len(ec.Code) != 8 {
		t.Fatalf("code = %q", ec.Code)
	}

	certPEM, fp := testCert(t)
	code, raw = env.do(t, http.MethodPost, "/agent/v1/enroll", "", map[string]any{
		"code": ec.Code, "hostname": "box", "os": "linux", "arch": "amd64",
		"version": "test", "certPem": certPEM, "fingerprint": fp,
		"ips": []string{"10.0.0.5"},
	})
	if code != http.StatusOK {
		t.Fatalf("enroll = %d: %s", code, raw)
	}
	var enrolled struct {
		HostID string `json:"hostId"`
		Token  string `json:"token"`
	}
	_ = json.Unmarshal(raw, &enrolled)

	// Reusing the one-time code must fail.
	code, _ = env.do(t, http.MethodPost, "/agent/v1/enroll", "", map[string]any{
		"code": ec.Code, "certPem": certPEM, "fingerprint": fp,
	})
	if code != http.StatusBadRequest {
		t.Fatalf("code reuse = %d", code)
	}

	code, raw = env.do(t, http.MethodPost, "/agent/v1/heartbeat", enrolled.Token, map[string]any{
		"hostId": enrolled.HostID, "version": "test", "uptimeSec": 10,
	})
	if code != http.StatusOK {
		t.Fatalf("heartbeat = %d: %s", code, raw)
	}
	code, _ = env.do(t, http.MethodPost, "/agent/v1/heartbeat", "wrong-token", map[string]any{
		"hostId": enrolled.HostID,
	})
	if code != http.StatusUnauthorized {
		t.Fatalf("bad heartbeat token = %d", code)
	}

	code, raw = env.do(t, http.MethodGet, "/api/v1/hosts/"+host.ID, env.token, nil)
	if code != http.StatusOK {
		t.Fatalf("get host = %d", code)
	}
	var got store.Host
	_ = json.Unmarshal(raw, &got)
	if got.Status != status.Online || !got.Capability.Shutdown {
		t.Fatalf("host = status %q caps %+v", got.Status, got.Capability)
	}
	stored, err := env.st.GetHost(host.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored.AgentURL, "10.0.0.5:45871") {
		t.Fatalf("agent URL = %q, want the reported LAN IP", stored.AgentURL)
	}
}

func TestEnrollFromLAN(t *testing.T) {
	s := &Server{trusted: parseTrusted([]string{"192.168.1.3/32"})}
	req := httptest.NewRequest(http.MethodPost, "/agent/v1/enroll", nil)
	req.RemoteAddr = "192.168.1.11:1234"
	if !s.enrollFromLAN(req) {
		t.Fatal("direct LAN enroll rejected")
	}
	req.RemoteAddr = "8.8.8.8:443"
	if s.enrollFromLAN(req) {
		t.Fatal("public enroll allowed")
	}
	req.RemoteAddr = "192.168.1.3:8085"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	if s.enrollFromLAN(req) {
		t.Fatal("public client via proxy allowed")
	}
	req.Header.Set("X-Forwarded-For", "192.168.1.20")
	if !s.enrollFromLAN(req) {
		t.Fatal("LAN client via proxy rejected")
	}
}

func TestChooseAgentIP(t *testing.T) {
	cases := []struct {
		name     string
		reported []string
		fallback string
		server   string
		want     string
	}{
		{"same subnet wins over docker", []string{"172.17.0.1", "10.255.255.254", "192.168.1.20"}, "172.17.0.1", "192.168.1.3:8085", "192.168.1.20"},
		{"skips loopback", []string{"127.0.0.1", "10.1.2.3"}, "172.17.0.1", "192.168.1.3:8085", "10.1.2.3"},
		{"falls back to public", []string{"1.2.3.4"}, "172.17.0.1", "0.0.0.0:8085", "1.2.3.4"},
		{"no data", nil, "192.168.1.9", "192.168.1.3:8085", "192.168.1.9"},
	}
	for _, c := range cases {
		if got := chooseAgentIP(c.reported, c.fallback, c.server); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func testCert(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-agent"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(der)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), hex.EncodeToString(sum[:])
}
