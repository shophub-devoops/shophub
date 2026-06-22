package grafana

import (
	"strings"
	"testing"
)

// TestPasswordEncryptionRoundTrip guards the at-rest encryption (the stored
// Grafana password must round-trip but never sit in the DB as plaintext, and
// must not decrypt under a different key).
func TestPasswordEncryptionRoundTrip(t *testing.T) {
	secret := []byte("test-secret")
	const pw = "s3cr3t-grafana-pw"

	enc, err := EncryptPassword(secret, pw)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if strings.Contains(enc, pw) {
		t.Fatalf("ciphertext leaks plaintext: %q", enc)
	}

	got, err := DecryptPassword(secret, enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != pw {
		t.Fatalf("round-trip = %q, want %q", got, pw)
	}

	if _, err := DecryptPassword([]byte("other-secret"), enc); err == nil {
		t.Fatal("decrypt with wrong key should fail")
	}
}
