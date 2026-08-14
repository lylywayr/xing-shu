package api

import (
	"net/http/httptest"
	"testing"
	"xing-shu/internal/auth"
)

func TestAdminProtected(t *testing.T) {
	s := &Server{Auth: auth.Authorizer{AdminKey: "secret"}}
	r := httptest.NewRequest("GET", "/api/admin/models", nil)
	w := httptest.NewRecorder()
	s.Routes().ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("want 401 got %d", w.Code)
	}
}

func TestAdminAuthorized(t *testing.T) {
	s := &Server{Auth: auth.Authorizer{AdminKey: "secret"}}
	r := httptest.NewRequest("GET", "/api/admin/models", nil)
	r.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	s.Routes().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("want 200 got %d", w.Code)
	}
}
