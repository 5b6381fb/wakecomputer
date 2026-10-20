package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpired      = errors.New("token expired")
)

// ---------- password hashing ----------

const (
	// OWASP minimum for Argon2id (19 MiB, t=2, p=1). Keeping the memory cost
	// modest matters because this runs on a router-class device.
	argonTime    = 2
	argonMemory  = 19 * 1024
	argonThreads = 1
	argonKeyLen  = 32
)

func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var m uint32
	var t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// DummyHash is a syntactically valid argon2id hash used to keep login timing
// roughly constant when the username does not exist.
var DummyHash = fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
	argonMemory, argonTime, argonThreads,
	base64.RawStdEncoding.EncodeToString(make([]byte, 16)),
	base64.RawStdEncoding.EncodeToString(make([]byte, 32)))

// ---------- opaque tokens ----------

func NewToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ---------- JWT (HS256) ----------

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type jwtClaims struct {
	Sub string `json:"sub"`
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
	Jti string `json:"jti"`
}

func SignJWT(secret, subject string, ttl time.Duration) (string, int64, error) {
	now := time.Now()
	exp := now.Add(ttl).Unix()
	h, _ := json.Marshal(jwtHeader{Alg: "HS256", Typ: "JWT"})
	c, _ := json.Marshal(jwtClaims{
		Sub: subject,
		Iat: now.Unix(),
		Exp: exp,
		Jti: base64.RawURLEncoding.EncodeToString(randomBytes(12)),
	})
	body := b64(h) + "." + b64(c)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return body + "." + b64(mac.Sum(nil)), exp, nil
}

func VerifyJWT(secret, token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", ErrInvalidToken
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", ErrInvalidToken
	}
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return "", ErrInvalidToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ErrInvalidToken
	}
	var c jwtClaims
	if err := json.Unmarshal(raw, &c); err != nil {
		return "", ErrInvalidToken
	}
	if c.Exp > 0 && time.Now().Unix() > c.Exp {
		return "", ErrExpired
	}
	if c.Sub == "" {
		return "", ErrInvalidToken
	}
	return c.Sub, nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

// ---------- login rate limiting ----------

type bucket struct {
	tokens float64
	last   time.Time
}

type Limiter struct {
	mu      sync.Mutex
	perMin  float64
	burst   float64
	buckets map[string]*bucket
}

func NewLimiter(perMin, burst float64) *Limiter {
	return &Limiter{perMin: perMin, burst: burst, buckets: map[string]*bucket{}}
}

func (l *Limiter) Allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.buckets) > 20000 {
		for k, b := range l.buckets {
			if now.Sub(b.last) > 10*time.Minute {
				delete(l.buckets, k)
			}
		}
	}
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens += elapsed * l.perMin / 60
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

type lockState struct {
	fails       int
	lockedUntil time.Time
}

type Guard struct {
	mu    sync.Mutex
	state map[string]*lockState
	max   int
	lock  time.Duration
}

func NewGuard(maxFails int, lockFor time.Duration) *Guard {
	return &Guard{state: map[string]*lockState{}, max: maxFails, lock: lockFor}
}

func (g *Guard) Check(key string) (bool, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	s, ok := g.state[key]
	if !ok {
		return false, 0
	}
	if time.Now().Before(s.lockedUntil) {
		return true, time.Until(s.lockedUntil)
	}
	return false, 0
}

func (g *Guard) Fail(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	s, ok := g.state[key]
	if !ok {
		s = &lockState{}
		g.state[key] = s
	}
	s.fails++
	if s.fails >= g.max {
		s.lockedUntil = time.Now().Add(g.lock)
		s.fails = 0
	}
}

func (g *Guard) Reset(key string) {
	g.mu.Lock()
	delete(g.state, key)
	g.mu.Unlock()
}

func RetryAfterSeconds(d time.Duration) string {
	n := int(d.Seconds())
	if n < 1 {
		n = 1
	}
	return strconv.Itoa(n)
}
