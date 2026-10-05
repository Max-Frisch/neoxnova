package auth

import (
	"strings"
	"testing"
)

func TestHashPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hash, "pbkdf2_sha256$") {
		t.Fatalf("unexpected hash prefix: %q", hash)
	}
	if !VerifyPassword("correct horse battery staple", hash) {
		t.Fatal("correct password rejected")
	}
	if VerifyPassword("wrong password", hash) {
		t.Fatal("wrong password accepted")
	}
}

func TestHashPasswordUniqueSalt(t *testing.T) {
	a, err := HashPassword("same-password")
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashPassword("same-password")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("identical passwords produced identical hashes (salt not random)")
	}
}

func TestVerifyPasswordRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"", "garbage", "pbkdf2_sha256$notanint$a$b", "pbkdf2_sha256$1000$$", "argon2id$x$y$z"} {
		if VerifyPassword("x", bad) {
			t.Fatalf("malformed hash accepted: %q", bad)
		}
	}
}

func TestSessionToken(t *testing.T) {
	raw, hash, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if raw == "" || hash == "" || raw == hash {
		t.Fatalf("bad token: raw=%q hash=%q", raw, hash)
	}
	if HashToken(raw) != hash {
		t.Fatal("HashToken mismatch")
	}
	raw2, hash2, _ := NewSessionToken()
	if raw == raw2 || hash == hash2 {
		t.Fatal("session tokens are not unique")
	}
}
