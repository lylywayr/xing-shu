package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRootEndpointRejectsQueryCredentialsAndUnsafePort(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1:3001?x=1", "ftp://127.0.0.1:3001", "http://127.0.0.1:22", "http://user:pass@127.0.0.1:3001"} {
		if got := rootEndpoint(raw); got != "" {
			t.Fatalf("unsafe endpoint accepted: %q -> %q", raw, got)
		}
	}
	if got := rootEndpoint("http://127.0.0.1:3001/v1/"); got != "http://127.0.0.1:3001" {
		t.Fatalf("unexpected normalized endpoint: %q", got)
	}
}

func TestNewDefaultsToUnauthorizedAndPersistsOneTimeMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "integration.json")
	m, err := New(path, "http://127.0.0.1:3001/v1", "key", false)
	if err != nil || m.Authorized() {
		t.Fatalf("new integration must be unauthorized: err=%v state=%+v", err, m.State())
	}
	m, err = New(path, "http://127.0.0.1:3001/v1", "key", true)
	if err != nil || m.Authorized() {
		t.Fatalf("migration flag should not authorize an existing unauthorized record: err=%v state=%+v", err, m.State())
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestProbeDoesNotFollowRedirectAndBoundsPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/livez" {
			http.Redirect(w, r, "/other", http.StatusFound)
			return
		}
		t.Fatal("redirect was followed")
	}))
	defer server.Close()
	m, err := New(filepath.Join(t.TempDir(), "state.json"), server.URL, "key", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Probe(context.Background()); err == nil {
		t.Fatal("redirecting service must not pass liveness")
	}
}
