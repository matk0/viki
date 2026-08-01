package security_test

import (
	"bytes"
	"testing"

	"viki/internal/security"
)

func TestPasswordAndOpaqueTokenRoundTrip(t *testing.T) {
	hash, err := security.HashPassword("tajne-heslo")
	if err != nil {
		t.Fatal(err)
	}
	if !security.VerifyPassword(hash, "tajne-heslo") {
		t.Fatal("correct password was rejected")
	}
	if security.VerifyPassword(hash, "nespravne") {
		t.Fatal("incorrect password was accepted")
	}

	token, tokenHash, err := security.NewOpaqueToken()
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || len(tokenHash) == 0 {
		t.Fatal("empty token material")
	}
	if !bytes.Equal(tokenHash, security.HashToken(token)) {
		t.Fatal("stored token hash does not match presented token")
	}
}

func TestVerifyPasswordRejectsMalformedEncodings(t *testing.T) {
	tests := []string{
		"not-an-argon-hash",
		"$scrypt$v=19$m=65536,t=3,p=2$c2FsdA$a2V5",
		"$argon2id$v=18$m=65536,t=3,p=2$c2FsdA$a2V5",
		"$argon2id$v=19$invalid$c2FsdA$a2V5",
		"$argon2id$v=19$m=65536,t=3,p=2$%%%$a2V5",
		"$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$%%%",
	}

	for _, encoded := range tests {
		if security.VerifyPassword(encoded, "password") {
			t.Fatalf("malformed password encoding %q was accepted", encoded)
		}
	}
}
