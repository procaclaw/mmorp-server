package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPasswordHashAndVerify(t *testing.T) {
	h, err := hashPassword("supersecurepass")
	if err != nil {
		t.Fatalf("hashPassword err: %v", err)
	}
	ok, err := verifyPassword(h, "supersecurepass")
	if err != nil {
		t.Fatalf("verifyPassword err: %v", err)
	}
	if !ok {
		t.Fatal("expected password verification success")
	}
	ok, err = verifyPassword(h, "wrong-pass")
	if err != nil {
		t.Fatalf("verifyPassword wrong err: %v", err)
	}
	if ok {
		t.Fatal("expected password verification failure")
	}
}

func TestTokenIssueAndParse(t *testing.T) {
	s := &Service{jwtSecret: []byte("secret"), jwtTTL: time.Hour}
	uid := uuid.New()
	tok, err := s.issueToken(uid, "player@example.com")
	if err != nil {
		t.Fatalf("issueToken err: %v", err)
	}
	parsed, err := s.ParseToken(tok)
	if err != nil {
		t.Fatalf("ParseToken err: %v", err)
	}
	if parsed != uid {
		t.Fatalf("parsed uid mismatch: got %v want %v", parsed, uid)
	}
}

func TestRegisterValidation(t *testing.T) {
	s := &Service{jwtSecret: []byte("secret"), jwtTTL: time.Hour}
	_, err := s.Register(context.Background(), "not-an-email", "supersecurepass")
	if !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("expected ErrInvalidEmail, got %v", err)
	}

	_, err = s.Register(context.Background(), "player@example.com", "short")
	if !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("expected ErrWeakPassword, got %v", err)
	}
}
