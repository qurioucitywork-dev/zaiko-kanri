package database

import (
	"errors"
	"strings"
	"testing"
)

func TestGuestPasswordBoundaries(t *testing.T) {
	for _, password := range []string{"test1234", "test12345", "test123456", "test1234567", "guest-password", "あいうえおかきく", strings.Repeat("a", 72)} {
		hash, err := HashGuestPassword(password)
		if err != nil {
			t.Fatalf("length %d rejected: %v", len(password), err)
		}
		if !VerifyPassword(hash, password) || VerifyPassword(hash, "incorrect") {
			t.Fatal("password verification failed")
		}
	}
	for _, password := range []string{"", "1234567", "あいう", strings.Repeat("a", 73), strings.Repeat("あ", 25)} {
		if _, err := HashGuestPassword(password); !errors.Is(err, ErrGuestPasswordInvalid) {
			t.Fatal("invalid guest password accepted")
		}
	}
	if _, err := HashPassword("test1234"); err == nil {
		t.Fatal("staff password policy weakened")
	}
}
