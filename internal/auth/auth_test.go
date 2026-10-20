package auth

import (
	"strings"
	"testing"
	"time"
)

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("unexpected hash format: %s", hash)
	}
	if !VerifyPassword("correct horse battery staple", hash) {
		t.Fatal("valid password rejected")
	}
	if VerifyPassword("wrong", hash) {
		t.Fatal("invalid password accepted")
	}
	if VerifyPassword("anything", "$argon2id$broken") {
		t.Fatal("malformed hash accepted")
	}
}

func TestJWT(t *testing.T) {
	secret := strings.Repeat("s", 48)
	token, exp, err := SignJWT(secret, "user-1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if exp <= time.Now().Unix() {
		t.Fatal("expiry not in the future")
	}
	sub, err := VerifyJWT(secret, token)
	if err != nil || sub != "user-1" {
		t.Fatalf("VerifyJWT = %q, %v", sub, err)
	}
	if _, err := VerifyJWT("other-secret", token); err == nil {
		t.Fatal("token accepted with wrong secret")
	}
	if _, err := VerifyJWT(secret, token+"x"); err == nil {
		t.Fatal("tampered token accepted")
	}

	expired, _, _ := SignJWT(secret, "user-1", -time.Second)
	if _, err := VerifyJWT(secret, expired); err != ErrExpired {
		t.Fatalf("expired token err = %v", err)
	}
}

func TestLimiterAndGuard(t *testing.T) {
	l := NewLimiter(1, 2)
	if !l.Allow("k") || !l.Allow("k") {
		t.Fatal("burst rejected")
	}
	if l.Allow("k") {
		t.Fatal("limit not enforced")
	}

	g := NewGuard(2, time.Minute)
	if locked, _ := g.Check("u"); locked {
		t.Fatal("unexpected lock")
	}
	g.Fail("u")
	g.Fail("u")
	if locked, _ := g.Check("u"); !locked {
		t.Fatal("expected lock after max failures")
	}
	g.Reset("u")
	if locked, _ := g.Check("u"); locked {
		t.Fatal("reset did not clear lock")
	}
}
