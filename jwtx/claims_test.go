package jwtx

import (
	"testing"
	"time"
)

func TestClaimsRoundTrip(t *testing.T) {
	type payload struct {
		Role string `json:"role"`
	}
	issuedAt := time.Now().Add(-time.Minute)
	expiresAt := issuedAt.Add(time.Hour)
	claims := &Claims{
		ID:        "token-1",
		Subject:   "user-1",
		IssuedAt:  NewTime(issuedAt),
		ExpiresAt: NewTime(expiresAt),
		Extra:     map[string]any{"role": "admin"},
	}
	token, err := HS256("secret").Sign(claims)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	parsed := &Claims{}
	if err = HS256("secret").Parse(token, parsed); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.ID != "token-1" {
		t.Fatalf("unexpected id: %s", parsed.ID)
	}
	if parsed.Subject != "user-1" {
		t.Fatalf("unexpected subject: %s", parsed.Subject)
	}
	if !parsed.IssuedAt.Equal(issuedAt.Truncate(time.Second)) {
		t.Fatalf("unexpected issued at: %v", parsed.IssuedAt)
	}
	if !parsed.ExpiresAt.Equal(expiresAt.Truncate(time.Second)) {
		t.Fatalf("unexpected expires at: %v", parsed.ExpiresAt)
	}
	var value payload
	if err = parsed.Decode(&value); err != nil {
		t.Fatalf("decode extra: %v", err)
	}
	if value.Role != "admin" {
		t.Fatalf("unexpected extra role: %s", value.Role)
	}
}

func TestClaimsRejectsExpiredToken(t *testing.T) {
	now := time.Now()
	claims := &Claims{
		Subject:   "user-1",
		IssuedAt:  NewTime(now.Add(-2 * time.Hour)),
		ExpiresAt: NewTime(now.Add(-time.Hour)),
	}
	token, err := HS256("secret").Sign(claims)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err = HS256("secret").Parse(token, &Claims{}); err == nil {
		t.Fatal("expired token must be rejected")
	}
}
