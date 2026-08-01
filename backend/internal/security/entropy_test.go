package security

import (
	"errors"
	"testing"
)

func TestCredentialGenerationReportsEntropyFailure(t *testing.T) {
	original := readRandom
	readRandom = func([]byte) (int, error) {
		return 0, errors.New("entropy unavailable")
	}
	t.Cleanup(func() { readRandom = original })

	if _, err := HashPassword("password"); err == nil {
		t.Fatal("password hashing accepted an unavailable entropy source")
	}
	if _, _, err := NewOpaqueToken(); err == nil {
		t.Fatal("token generation accepted an unavailable entropy source")
	}
}
