package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// SessionTokenBytes is the entropy of a session token (256 bits).
const SessionTokenBytes = 32

// NewSessionToken returns a fresh opaque token and the hash to persist. The raw
// token is shown to the client once (in a cookie); only its hash is stored, so a
// database leak does not expose usable sessions.
func NewSessionToken() (raw string, hash string, err error) {
	b := make([]byte, SessionTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, HashToken(raw), nil
}

// HashToken returns the hex SHA-256 of a raw session token, for lookup.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
