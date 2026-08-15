package api

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"xing-shu/internal/auth"
	"xing-shu/internal/clientkey"
)

func TestClientKeyAdminLifecycleAndAPIInfo(t *testing.T) {
	store, _ := clientkey.Open(filepath.Join(t.TempDir(), "keys.json"))
	keys := &ClientKeys{Store: store}
	server := &Server{Auth: auth.Authorizer{AdminKey: "admin"}, ClientKeys: keys, ClientKeyStore: store, PublicBaseURL: "https://router.example"}
	create := httptest.NewRequest("POST", "/api/admin/client-keys/create", strings.NewReader(`{"name":"app","scopes":["models:read","chat:write"]}`))
	create.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	server.Routes().ServeHTTP(w, create)
	if w.Code != 200 {
		t.Fatalf("create failed: %d %s", w.Code, w.Body.String())
	}
	var payload map[string]any
	json.Unmarshal(w.Body.Bytes(), &payload)
	secret, _ := payload["secret"].(string)
	if secret == "" {
		t.Fatal("secret missing on creation")
	}
	list := httptest.NewRequest("GET", "/api/admin/client-keys", nil)
	list.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	server.Routes().ServeHTTP(w, list)
	if strings.Contains(w.Body.String(), secret) {
		t.Fatal("list leaked client secret")
	}
	info := httptest.NewRequest("GET", "/api/admin/client-api/info", nil)
	info.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	server.Routes().ServeHTTP(w, info)
	if !strings.Contains(w.Body.String(), "https://router.example/v1/chat/completions") {
		t.Fatalf("wrong API info: %s", w.Body.String())
	}
}
