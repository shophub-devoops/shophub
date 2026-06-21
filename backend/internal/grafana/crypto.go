package grafana

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Grafana credentials are stored encrypted at rest: the tenant's Grafana
// password must be shown back to them (so it can't be a one-way hash), but
// keeping it as plaintext in the users table would leak every tenant's Grafana
// access on a DB dump. AES-256-GCM with a key derived from the JWT signing
// secret — which the backend already holds — means no extra secret to manage.

// cipherKey derives a 32-byte AES key from the JWT signing secret. The prefix
// domain-separates it from the secret's other HMAC/JWT uses.
func cipherKey(secret []byte) [32]byte {
	return sha256.Sum256(append([]byte("shophub-grafana-pw:"), secret...))
}

// EncryptPassword seals plaintext with AES-256-GCM and returns hex(nonce || ciphertext).
func EncryptPassword(secret []byte, plaintext string) (string, error) {
	key := cipherKey(secret)
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return hex.EncodeToString(gcm.Seal(nonce, nonce, []byte(plaintext), nil)), nil
}

// DecryptPassword reverses EncryptPassword.
func DecryptPassword(secret []byte, encHex string) (string, error) {
	key := cipherKey(secret)
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	raw, err := hex.DecodeString(encHex)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	out, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func newGCM(key [32]byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
