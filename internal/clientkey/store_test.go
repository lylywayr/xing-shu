package clientkey

import (
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestKeyLifecycleAndOneTimeSecret(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "keys.json"))
	if err != nil {
		t.Fatal(err)
	}
	created, secret, err := s.Create(CreateInput{Name: "ci", Scopes: []string{"models:read", "chat:write"}})
	if err != nil || secret == "" {
		t.Fatalf("create: %v", err)
	}
	if created.Hash != "" {
		t.Fatal("hash leaked")
	}
	if got := s.List()[0]; got.Secret != "" || got.Hash != "" {
		t.Fatal("list leaked secret material")
	}
	req := httptest.NewRequest("GET", "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	key, err := s.Authenticate(req, "models:read", time.Now())
	if err != nil || key.ID != created.ID {
		t.Fatalf("authenticate: %v", err)
	}
	if err := s.Revoke(created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(req, "models:read", time.Now()); err == nil {
		t.Fatal("revoked key authenticated")
	}
}

func TestExpiredAndScopedKeysAreRejected(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "keys.json"))
	past := time.Now().Add(-time.Hour)
	_, secret, _ := s.Create(CreateInput{Name: "expired", Scopes: []string{"models:read"}, ExpiresAt: &past})
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	if _, err := s.Authenticate(req, "chat:write", time.Now()); err == nil {
		t.Fatal("expired/scoped key authenticated")
	}
}
