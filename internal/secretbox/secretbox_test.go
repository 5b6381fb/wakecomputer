package secretbox

import "testing"

func TestSealOpen(t *testing.T) {
	key := Key("server-secret", "agent-token")
	enc, err := Seal(key, "super-secret-token")
	if err != nil {
		t.Fatal(err)
	}
	if enc == "super-secret-token" {
		t.Fatal("plaintext leaked")
	}
	got, err := Open(key, enc)
	if err != nil || got != "super-secret-token" {
		t.Fatalf("Open = %q, %v", got, err)
	}
	if _, err := Open(Key("other", "agent-token"), enc); err == nil {
		t.Fatal("decrypted with wrong key")
	}
	if _, err := Open(key, "not-base64!"); err == nil {
		t.Fatal("invalid ciphertext accepted")
	}
}
