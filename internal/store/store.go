package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wakecomputer/wakecomputer/internal/id"
	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db *sql.DB
}

type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	TOTPSecret   string `json:"-"`
	CreatedAt    int64  `json:"createdAt"`
	UpdatedAt    int64  `json:"updatedAt"`
}

type Session struct {
	ID          string
	UserID      string
	RefreshHash string
	DeviceName  string
	UserAgent   string
	IP          string
	CreatedAt   int64
	LastUsedAt  int64
	ExpiresAt   int64
	RevokedAt   *int64
}

type Group struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sortOrder"`
}

type Host struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	OSType           string   `json:"osType"`
	MAC              string   `json:"mac"`
	IP               string   `json:"ip"`
	BroadcastTargets []string `json:"broadcastTargets"`
	WolPort          int      `json:"wolPort"`
	ProbePort        int      `json:"probePort"`
	Notes            string   `json:"notes"`
	SortOrder        int      `json:"sortOrder"`
	AgentURL         string   `json:"-"`
	AgentTokenEnc    string   `json:"-"`
	AgentFingerprint string   `json:"-"`
	AgentVersion     string   `json:"agentVersion,omitempty"`
	LastSeenAt       *int64   `json:"lastSeenAt,omitempty"`
	LastStatus       string   `json:"lastStatus"`
	Groups           []Group  `json:"groups"`
	CreatedAt        int64    `json:"createdAt"`
	UpdatedAt        int64    `json:"updatedAt"`

	// Runtime fields, not persisted.
	Status     string `json:"status"`
	Capability struct {
		Wake     bool `json:"wake"`
		Shutdown bool `json:"shutdown"`
		Reboot   bool `json:"reboot"`
		Cancel   bool `json:"cancel"`
	} `json:"capabilities"`
}

type Audit struct {
	ID        int64  `json:"id"`
	UserID    string `json:"userId,omitempty"`
	Username  string `json:"username,omitempty"`
	Action    string `json:"action"`
	HostID    string `json:"hostId,omitempty"`
	HostName  string `json:"hostName,omitempty"`
	Result    string `json:"result"`
	Detail    string `json:"detail,omitempty"`
	IP        string `json:"ip,omitempty"`
	UserAgent string `json:"userAgent,omitempty"`
	CreatedAt int64  `json:"createdAt"`
}

const schema = `
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  username TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  totp_secret TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  refresh_hash TEXT NOT NULL UNIQUE,
  device_name TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  ip TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  last_used_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  revoked_at INTEGER,
  FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id, revoked_at);
CREATE TABLE IF NOT EXISTS hosts (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  os_type TEXT NOT NULL DEFAULT 'other',
  mac TEXT NOT NULL,
  ip TEXT NOT NULL DEFAULT '',
  broadcast_targets TEXT NOT NULL DEFAULT '[]',
  wol_port INTEGER NOT NULL DEFAULT 9,
  probe_port INTEGER NOT NULL DEFAULT 0,
  notes TEXT NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL DEFAULT 0,
  agent_url TEXT NOT NULL DEFAULT '',
  agent_token_enc TEXT NOT NULL DEFAULT '',
  agent_fingerprint TEXT NOT NULL DEFAULT '',
  agent_version TEXT NOT NULL DEFAULT '',
  last_seen_at INTEGER,
  last_status TEXT NOT NULL DEFAULT 'unknown',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS groups (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  sort_order INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS host_groups (
  host_id TEXT NOT NULL,
  group_id TEXT NOT NULL,
  PRIMARY KEY(host_id, group_id),
  FOREIGN KEY(host_id) REFERENCES hosts(id) ON DELETE CASCADE,
  FOREIGN KEY(group_id) REFERENCES groups(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS enrollment_codes (
  code_hash TEXT PRIMARY KEY,
  host_id TEXT NOT NULL,
  expires_at INTEGER NOT NULL,
  used_at INTEGER
);
CREATE TABLE IF NOT EXISTS audit_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL,
  host_id TEXT NOT NULL DEFAULT '',
  result TEXT NOT NULL,
  detail TEXT NOT NULL DEFAULT '',
  ip TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_time ON audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_host ON audit_logs(host_id, created_at DESC);
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`

func Open(path string) (*Store, error) {
	dsn := "file:" + path +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(time.Hour)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }
func (s *Store) DB() *sql.DB  { return s.db }

func now() int64 { return time.Now().UnixMilli() }

// ---------- users ----------

func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) CreateUser(username, passwordHash string) (*User, error) {
	u := &User{
		ID:           id.New(),
		Username:     username,
		PasswordHash: passwordHash,
		CreatedAt:    now(),
		UpdatedAt:    now(),
	}
	_, err := s.db.Exec(`INSERT INTO users(id,username,password_hash,created_at,updated_at) VALUES(?,?,?,?,?)`,
		u.ID, u.Username, u.PasswordHash, u.CreatedAt, u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Store) GetUserByUsername(username string) (*User, error) {
	u := &User{}
	err := s.db.QueryRow(`SELECT id,username,password_hash,totp_secret,created_at,updated_at FROM users WHERE username=?`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.TOTPSecret, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

func (s *Store) GetUserByID(id string) (*User, error) {
	u := &User{}
	err := s.db.QueryRow(`SELECT id,username,password_hash,totp_secret,created_at,updated_at FROM users WHERE id=?`, id).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.TOTPSecret, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

func (s *Store) UpdatePassword(userID, hash string) error {
	_, err := s.db.Exec(`UPDATE users SET password_hash=?, updated_at=? WHERE id=?`, hash, now(), userID)
	return err
}

// ---------- sessions ----------

func (s *Store) CreateSession(sess *Session) error {
	_, err := s.db.Exec(`INSERT INTO sessions(id,user_id,refresh_hash,device_name,user_agent,ip,created_at,last_used_at,expires_at)
		VALUES(?,?,?,?,?,?,?,?,?)`,
		sess.ID, sess.UserID, sess.RefreshHash, sess.DeviceName, sess.UserAgent, sess.IP,
		sess.CreatedAt, sess.LastUsedAt, sess.ExpiresAt)
	return err
}

func (s *Store) GetSessionByHash(hash string) (*Session, error) {
	sess := &Session{}
	err := s.db.QueryRow(`SELECT id,user_id,refresh_hash,device_name,user_agent,ip,created_at,last_used_at,expires_at,revoked_at
		FROM sessions WHERE refresh_hash=?`, hash).
		Scan(&sess.ID, &sess.UserID, &sess.RefreshHash, &sess.DeviceName, &sess.UserAgent,
			&sess.IP, &sess.CreatedAt, &sess.LastUsedAt, &sess.ExpiresAt, &sess.RevokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return sess, err
}

func (s *Store) TouchSession(id string) error {
	_, err := s.db.Exec(`UPDATE sessions SET last_used_at=? WHERE id=?`, now(), id)
	return err
}

func (s *Store) RevokeSession(id string) error {
	_, err := s.db.Exec(`UPDATE sessions SET revoked_at=? WHERE id=? AND revoked_at IS NULL`, now(), id)
	return err
}

func (s *Store) RevokeUserSessions(userID, exceptID string) error {
	_, err := s.db.Exec(`UPDATE sessions SET revoked_at=? WHERE user_id=? AND id<>? AND revoked_at IS NULL`, now(), userID, exceptID)
	return err
}

func (s *Store) ListSessions(userID string) ([]Session, error) {
	rows, err := s.db.Query(`SELECT id,user_id,device_name,user_agent,ip,created_at,last_used_at,expires_at,revoked_at
		FROM sessions WHERE user_id=? ORDER BY last_used_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var x Session
		if err := rows.Scan(&x.ID, &x.UserID, &x.DeviceName, &x.UserAgent, &x.IP, &x.CreatedAt, &x.LastUsedAt, &x.ExpiresAt, &x.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// ---------- hosts ----------

func (s *Store) ListHosts() ([]*Host, error) {
	rows, err := s.db.Query(`SELECT id,name,os_type,mac,ip,broadcast_targets,wol_port,probe_port,notes,sort_order,
		agent_url,agent_token_enc,agent_fingerprint,agent_version,last_seen_at,last_status,created_at,updated_at
		FROM hosts ORDER BY sort_order, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hosts []*Host
	index := map[string]*Host{}
	for rows.Next() {
		h := &Host{}
		var bt string
		if err := rows.Scan(&h.ID, &h.Name, &h.OSType, &h.MAC, &h.IP, &bt, &h.WolPort, &h.ProbePort,
			&h.Notes, &h.SortOrder, &h.AgentURL, &h.AgentTokenEnc, &h.AgentFingerprint, &h.AgentVersion,
			&h.LastSeenAt, &h.LastStatus, &h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(bt), &h.BroadcastTargets)
		h.Groups = []Group{}
		hosts = append(hosts, h)
		index[h.ID] = h
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	grows, err := s.db.Query(`SELECT hg.host_id,g.id,g.name,g.sort_order
		FROM host_groups hg JOIN groups g ON g.id=hg.group_id ORDER BY g.sort_order, g.name`)
	if err != nil {
		return nil, err
	}
	defer grows.Close()
	for grows.Next() {
		var hostID string
		var g Group
		if err := grows.Scan(&hostID, &g.ID, &g.Name, &g.SortOrder); err != nil {
			return nil, err
		}
		if h, ok := index[hostID]; ok {
			h.Groups = append(h.Groups, g)
		}
	}
	return hosts, grows.Err()
}

const hostCols = `id,name,os_type,mac,ip,broadcast_targets,wol_port,probe_port,notes,sort_order,
agent_url,agent_token_enc,agent_fingerprint,agent_version,last_seen_at,last_status,created_at,updated_at`

func scanHost(row interface{ Scan(...any) error }) (*Host, error) {
	h := &Host{}
	var bt string
	err := row.Scan(&h.ID, &h.Name, &h.OSType, &h.MAC, &h.IP, &bt, &h.WolPort, &h.ProbePort,
		&h.Notes, &h.SortOrder, &h.AgentURL, &h.AgentTokenEnc, &h.AgentFingerprint, &h.AgentVersion,
		&h.LastSeenAt, &h.LastStatus, &h.CreatedAt, &h.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(bt), &h.BroadcastTargets)
	h.Groups = []Group{}
	return h, nil
}

func (s *Store) GetHost(id string) (*Host, error) {
	h, err := scanHost(s.db.QueryRow(`SELECT `+hostCols+` FROM hosts WHERE id=?`, id))
	if err != nil {
		return nil, err
	}
	grows, err := s.db.Query(`SELECT g.id,g.name,g.sort_order FROM host_groups hg
		JOIN groups g ON g.id=hg.group_id WHERE hg.host_id=? ORDER BY g.sort_order,g.name`, id)
	if err != nil {
		return nil, err
	}
	defer grows.Close()
	for grows.Next() {
		var g Group
		if err := grows.Scan(&g.ID, &g.Name, &g.SortOrder); err != nil {
			return nil, err
		}
		h.Groups = append(h.Groups, g)
	}
	return h, grows.Err()
}

func (s *Store) CreateHost(h *Host) error {
	bt, _ := json.Marshal(h.BroadcastTargets)
	ts := now()
	h.CreatedAt, h.UpdatedAt = ts, ts
	if h.LastStatus == "" {
		h.LastStatus = "unknown"
	}
	_, err := s.db.Exec(`INSERT INTO hosts(id,name,os_type,mac,ip,broadcast_targets,wol_port,probe_port,notes,sort_order,last_status,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		h.ID, h.Name, h.OSType, h.MAC, h.IP, string(bt), h.WolPort, h.ProbePort, h.Notes, h.SortOrder, h.LastStatus, h.CreatedAt, h.UpdatedAt)
	return err
}

func (s *Store) UpdateHost(h *Host) error {
	bt, _ := json.Marshal(h.BroadcastTargets)
	h.UpdatedAt = now()
	_, err := s.db.Exec(`UPDATE hosts SET name=?,os_type=?,mac=?,ip=?,broadcast_targets=?,wol_port=?,probe_port=?,notes=?,sort_order=?,updated_at=?
		WHERE id=?`,
		h.Name, h.OSType, h.MAC, h.IP, string(bt), h.WolPort, h.ProbePort, h.Notes, h.SortOrder, h.UpdatedAt, h.ID)
	return err
}

func (s *Store) DeleteHost(id string) error {
	res, err := s.db.Exec(`DELETE FROM hosts WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetHostAgent(hostID, url, tokenEnc, fingerprint string) error {
	_, err := s.db.Exec(`UPDATE hosts SET agent_url=?,agent_token_enc=?,agent_fingerprint=?,updated_at=? WHERE id=?`,
		url, tokenEnc, fingerprint, now(), hostID)
	return err
}

func (s *Store) ClearHostAgent(hostID string) error {
	_, err := s.db.Exec(`UPDATE hosts SET agent_url='',agent_token_enc='',agent_fingerprint='',agent_version='',last_seen_at=NULL,last_status='unknown',updated_at=? WHERE id=?`,
		now(), hostID)
	return err
}

func (s *Store) UpdateHostStatus(hostID, status string) error {
	_, err := s.db.Exec(`UPDATE hosts SET last_status=? WHERE id=?`, status, hostID)
	return err
}

func (s *Store) SetHostIPIfEmpty(hostID, ip string) error {
	_, err := s.db.Exec(`UPDATE hosts SET ip=? WHERE id=? AND ip=''`, ip, hostID)
	return err
}

func (s *Store) UpdateHeartbeat(hostID, version, ip, agentURL string) error {
	ts := now()
	_, err := s.db.Exec(`UPDATE hosts SET agent_version=?,last_seen_at=?,last_status='online',
		ip=CASE WHEN ?!='' THEN ? ELSE ip END,
		agent_url=CASE WHEN ?!='' THEN ? ELSE agent_url END,
		updated_at=? WHERE id=?`, version, ts, ip, ip, agentURL, agentURL, ts, hostID)
	return err
}

func (s *Store) SetHostGroups(hostID string, groupIDs []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM host_groups WHERE host_id=?`, hostID); err != nil {
		return err
	}
	for _, gid := range groupIDs {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO host_groups(host_id,group_id) VALUES(?,?)`, hostID, gid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------- groups ----------

func (s *Store) ListGroups() ([]Group, error) {
	rows, err := s.db.Query(`SELECT id,name,sort_order FROM groups ORDER BY sort_order,name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) CreateGroup(name string, order int) (*Group, error) {
	g := &Group{ID: id.New(), Name: name, SortOrder: order}
	_, err := s.db.Exec(`INSERT INTO groups(id,name,sort_order) VALUES(?,?,?)`, g.ID, g.Name, g.SortOrder)
	return g, err
}

func (s *Store) UpdateGroup(id, name string, order int) error {
	res, err := s.db.Exec(`UPDATE groups SET name=?,sort_order=? WHERE id=?`, name, order, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteGroup(id string) error {
	res, err := s.db.Exec(`DELETE FROM groups WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- enrollment ----------

func (s *Store) CreateEnrollment(codeHash, hostID string, expiresAt int64) error {
	_, err := s.db.Exec(`INSERT INTO enrollment_codes(code_hash,host_id,expires_at) VALUES(?,?,?)`, codeHash, hostID, expiresAt)
	return err
}

func (s *Store) ConsumeEnrollment(codeHash string) (string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var hostID string
	var expiresAt int64
	var usedAt *int64
	err = tx.QueryRow(`SELECT host_id,expires_at,used_at FROM enrollment_codes WHERE code_hash=?`, codeHash).
		Scan(&hostID, &expiresAt, &usedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if usedAt != nil || now() > expiresAt {
		return "", ErrNotFound
	}
	if _, err := tx.Exec(`UPDATE enrollment_codes SET used_at=? WHERE code_hash=?`, now(), codeHash); err != nil {
		return "", err
	}
	return hostID, tx.Commit()
}

func (s *Store) CleanupExpired(olderThan int64) {
	_, _ = s.db.Exec(`DELETE FROM enrollment_codes WHERE expires_at < ?`, olderThan)
}

// ---------- audit ----------

func (s *Store) InsertAudit(a *Audit) error {
	if a.CreatedAt == 0 {
		a.CreatedAt = now()
	}
	res, err := s.db.Exec(`INSERT INTO audit_logs(user_id,action,host_id,result,detail,ip,user_agent,created_at)
		VALUES(?,?,?,?,?,?,?,?)`,
		a.UserID, a.Action, a.HostID, a.Result, a.Detail, a.IP, a.UserAgent, a.CreatedAt)
	if err != nil {
		return err
	}
	a.ID, _ = res.LastInsertId()
	return nil
}

type AuditFilter struct {
	Limit  int
	Cursor int64
	HostID string
	Action string
	Result string
	From   int64
	To     int64
}

func (s *Store) ListAudit(f AuditFilter) ([]Audit, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	q := `SELECT a.id,a.user_id,COALESCE(u.username,''),a.action,a.host_id,COALESCE(h.name,''),a.result,a.detail,a.ip,a.user_agent,a.created_at
		FROM audit_logs a
		LEFT JOIN users u ON u.id=a.user_id
		LEFT JOIN hosts h ON h.id=a.host_id
		WHERE 1=1`
	var args []any
	if f.Cursor > 0 {
		q += ` AND a.id < ?`
		args = append(args, f.Cursor)
	}
	if f.HostID != "" {
		q += ` AND a.host_id = ?`
		args = append(args, f.HostID)
	}
	if f.Action != "" {
		q += ` AND a.action = ?`
		args = append(args, f.Action)
	}
	if f.Result != "" {
		q += ` AND a.result = ?`
		args = append(args, f.Result)
	}
	if f.From > 0 {
		q += ` AND a.created_at >= ?`
		args = append(args, f.From)
	}
	if f.To > 0 {
		q += ` AND a.created_at <= ?`
		args = append(args, f.To)
	}
	q += ` ORDER BY a.id DESC LIMIT ?`
	args = append(args, f.Limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Audit
	for rows.Next() {
		var a Audit
		if err := rows.Scan(&a.ID, &a.UserID, &a.Username, &a.Action, &a.HostID, &a.HostName,
			&a.Result, &a.Detail, &a.IP, &a.UserAgent, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) PurgeAudit(before int64) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM audit_logs WHERE created_at < ?`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ---------- settings ----------

func (s *Store) GetSetting(key, def string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return def, nil
	}
	return v, err
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (s *Store) Backup(dest string) error {
	// VACUUM INTO produces a consistent snapshot even while WAL is active.
	_, err := s.db.Exec(`VACUUM INTO ?`, dest)
	return err
}

func NormalizeMAC(mac string) string {
	mac = strings.TrimSpace(strings.ToUpper(mac))
	return strings.ReplaceAll(mac, "-", ":")
}
