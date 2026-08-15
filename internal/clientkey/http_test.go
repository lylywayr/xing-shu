package clientkey

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestMiddlewareOptionalThenRequired(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "keys.json"))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	handler := Authenticator{Store: store}.Middleware("models:read", next)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/v1/models", nil))
	if w.Code != 204 || w.Header().Get("X-Xing-Shu-Auth") != "migration-optional" {
		t.Fatalf("optional mode failed: %d", w.Code)
	}
	if err := store.SetMode(ModeRequired); err == nil {
		t.Fatal("required mode enabled without a key")
	}
	_, secret, _ := store.Create(CreateInput{Name: "app", Scopes: []string{"models:read"}})
	if err := store.SetMode(ModeRequired); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/v1/models", nil))
	if w.Code != 401 {
		t.Fatalf("required mode accepted anonymous: %d", w.Code)
	}
	req := httptest.NewRequest("GET", "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Fatalf("valid key rejected: %d", w.Code)
	}
}

func TestHashPersistsButNeverLists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.json")
	store, _ := Open(path)
	created, secret, _ := store.Create(CreateInput{Name: "app", Scopes: []string{"chat:write"}})
	reloaded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	if _, err = reloaded.Authenticate(req, "chat:write", created.CreatedAt.Add(1)); err != nil {
		t.Fatal(err)
	}
	if reloaded.List()[0].Hash != "" {
		t.Fatal("hash exposed by list")
	}
}
