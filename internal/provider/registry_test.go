package provider

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testRegistry(t *testing.T) (*Registry, string) {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "providers-xing-shu.json")
	r, err := NewRegistry(path, base64.RawStdEncoding.EncodeToString(key), nil)
	if err != nil {
		t.Fatal(err)
	}
	return r, path
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		"https://api.example.com":       "https://api.example.com/v1",
		"https://api.example.com/":      "https://api.example.com/v1",
		"https://api.example.com/v1/":   "https://api.example.com/v1",
		"https://api.example.com/v1/v1": "https://api.example.com/v1",
	}
	for input, want := range cases {
		got, err := NormalizeBaseURL(input)
		if err != nil || got != want {
			t.Fatalf("NormalizeBaseURL(%q)=%q,%v want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"", "ftp://example.com", "https://user:pass@example.com", "https://example.com/v2"} {
		if _, err := NormalizeBaseURL(input); err == nil {
			t.Fatalf("expected %q to fail", input)
		}
	}
}

func TestRegistryEncryptsAndMasksCredentials(t *testing.T) {
	r, path := testRegistry(t)
	created, err := r.Create(ProviderInput{ID: "demo", Name: "Demo", BaseURL: "https://api.example.com", APIKey: "sk-super-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if created.APIKeyMask == "" || strings.Contains(created.APIKeyMask, "super") {
		t.Fatalf("unsafe mask: %q", created.APIKeyMask)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sk-super-secret") {
		t.Fatal("plaintext API key persisted")
	}
	config, ok := r.Config("demo")
	if !ok || config.APIKey != "sk-super-secret" || config.BaseURL != "https://api.example.com/v1" {
		t.Fatalf("bad config: %+v", config)
	}
	reloaded, err := NewRegistry(path, r.masterKeyText(), nil)
	if err != nil {
		t.Fatal(err)
	}
	config, ok = reloaded.Config("demo")
	if !ok || config.APIKey != "sk-super-secret" {
		t.Fatal("encrypted key did not survive reload")
	}
}

func TestRegistryCRUDAndEnvironmentReadOnly(t *testing.T) {
	env := map[string]Config{"env-one": {ID: "env-one", Name: "Env One", BaseURL: "https://env.example/v1", APIKey: "env-secret", Kind: "standard", Source: SourceEnvironment, Enabled: true}}
	r, _ := testRegistry(t)
	r.SetEnvironment(env)
	if _, err := r.Update("env-one", ProviderInput{Name: "changed"}); err != ErrReadOnly {
		t.Fatalf("expected readonly, got %v", err)
	}
	if err := r.Delete("env-one"); err != ErrReadOnly {
		t.Fatalf("expected readonly delete, got %v", err)
	}
	if _, err := r.Create(ProviderInput{ID: "dyn", Name: "Dynamic", BaseURL: "https://dyn.example/v1", APIKey: "secret"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Update("dyn", ProviderInput{Name: "Updated", BaseURL: "https://new.example", Enabled: boolPtr(false)}); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Config("dyn"); ok {
		t.Fatal("disabled provider must not be active")
	}
	if err := r.Delete("dyn"); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Get("dyn"); ok {
		t.Fatal("deleted provider remains")
	}
}

func boolPtr(v bool) *bool { return &v }
