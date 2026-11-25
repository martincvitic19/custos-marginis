package auth

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestHashVerify(t *testing.T) {
	h := Hash("correct horse battery")
	if !strings.HasPrefix(h, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Fatalf("unexpected encoding %q", h)
	}
	if !Verify("correct horse battery", h) || Verify("wrong", h) {
		t.Fatal("verify mismatch")
	}
}

func TestSetupLoginRateLimit(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "auth.json"))
	if s.IsSet() {
		t.Fatal("fresh store has a password")
	}
	if err := s.Setup("short"); err != ErrWeak {
		t.Fatalf("weak password accepted: %v", err)
	}
	if err := s.Setup("a long password"); err != nil {
		t.Fatal(err)
	}
	if err := s.Setup("another long one"); err != ErrAlreadySet {
		t.Fatal("second setup allowed")
	}
	for i := 0; i < 5; i++ {
		if _, err := s.Login("10.0.0.1", "nope"); err != ErrBadPassword {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if _, err := s.Login("10.0.0.1", "a long password"); err != ErrTooMany {
		t.Fatalf("6th attempt not rate limited: %v", err)
	}
	sess, err := s.Login("10.0.0.2", "a long password")
	if err != nil || s.Lookup(sess.Token) == nil || sess.CSRF == "" {
		t.Fatalf("login from another address: %v", err)
	}
}
