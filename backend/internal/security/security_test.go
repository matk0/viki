package security_test

import (
	"bytes"
	"testing"

	"viki/internal/security"
)

func TestPasswordAndOpaqueTokenRoundTrip(t *testing.T) {
	t.Parallel()

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
