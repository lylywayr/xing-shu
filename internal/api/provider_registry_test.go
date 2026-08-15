package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"xing-shu/internal/auth"
	"xing-shu/internal/catalog"
	"xing-shu/internal/provider"
)

func registryServer(t *testing.T, upstream *httptest.Server) (*Server, *provider.Registry) {
	t.Helper()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	registry, err := provider.NewRegistry(filepath.Join(t.TempDir(), "providers.json"), base64.RawStdEncoding.EncodeToString(key), nil)
	if err != nil {
		t.Fatal(err)
	}
	manager := catalog.NewManager(catalog.Catalog{}, nil)
	runtime := &ProviderRegistryRuntime{Registry: registry, Manager: manager}
	return &Server{Auth: auth.Authorizer{AdminKey: "secret"}, Manager: manager, ProviderRegistry: runtime}, registry
}
func registryRequest(s *Server, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer secret")
	r.Header.Set("Content-Type", "application/json")
	s.Routes().ServeHTTP(w, r)
	return w
}

func TestProviderRegistryAPICreateAndDelete(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": "m1"}}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]any{"content": "OK"}}}})
	}))
	defer upstream.Close()
	s, registry := registryServer(t, upstream)
	body := `{"id":"demo","name":"Demo","base_url":"` + upstream.URL + `","api_key":"secret-key","kind":"standard"}`
	w := registryRequest(s, "/api/admin/provider-registry/create", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "secret-key") {
		t.Fatal("secret leaked")
	}
	if _, ok := registry.Config("demo"); !ok {
		t.Fatal("provider not active")
	}
	w = registryRequest(s, "/api/admin/provider-registry/delete?id=demo", `{}`)
	if w.Code != 200 {
		t.Fatalf("delete %d %s", w.Code, w.Body.String())
	}
	if _, ok := registry.Get("demo"); ok {
		t.Fatal("provider remains")
	}
}
func TestProviderRegistryAPIRejectsUnverifiedCreate(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "unauthorized", 401) }))
	defer upstream.Close()
	s, _ := registryServer(t, upstream)
	w := registryRequest(s, "/api/admin/provider-registry/create", `{"id":"demo","name":"Demo","base_url":"`+upstream.URL+`","api_key":"bad"}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d %s", w.Code, w.Body.String())
	}
}
