package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestLoginUsesConfiguredCredentials(t *testing.T) {
	a := Authorizer{AdminKey: "key", Username: "admin", Password: "secret"}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", nil)
	req.Form = url.Values{"username": {"admin"}, "password": {"secret"}}
	res := httptest.NewRecorder()
	if !a.Login(res, req) {
		t.Fatal("configured credentials must authenticate")
	}
	if res.Header().Get("Set-Cookie") == "" {
		t.Fatal("login must issue a session cookie")
	}
}

func TestLoginRejectsMissingConfiguredCredentials(t *testing.T) {
	a := Authorizer{AdminKey: "key"}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", nil)
	req.Form = url.Values{"username": {"test-user"}, "password": {"test-pass"}}
	if a.Login(httptest.NewRecorder(), req) {
		t.Fatal("hardcoded historical credentials must not authenticate")
	}
}

func TestPasswordSessionWorksWithoutAdminAPIKey(t *testing.T) {
	a := Authorizer{Username: "admin", Password: "secret"}
	login := httptest.NewRequest(http.MethodPost, "/api/admin/login", nil)
	login.Form = url.Values{"username": {"admin"}, "password": {"secret"}}
	res := httptest.NewRecorder()
	if !a.Login(res, login) {
		t.Fatal("password login must authenticate")
	}
	cookie := res.Result().Cookies()[0]
	check := httptest.NewRequest(http.MethodGet, "/api/admin/models", nil)
	check.AddCookie(cookie)
	if !a.Check(check, Read) {
		t.Fatal("password session must authorize without admin key")
	}
}

func TestCheckAcceptsAnyValidDuplicateAdminCookie(t *testing.T) {
	a := Authorizer{Username: "admin", Password: "secret"}
	login := httptest.NewRequest(http.MethodPost, "/api/admin/login", nil)
	login.Form = url.Values{"username": {"admin"}, "password": {"secret"}}
	res := httptest.NewRecorder()
	if !a.Login(res, login) {
		t.Fatal("login failed")
	}
	valid := res.Result().Cookies()[0]
	r := httptest.NewRequest(http.MethodGet, "/api/admin/status", nil)
	r.Header.Add("Cookie", "xing_shu_admin=stale.invalid")
	r.AddCookie(valid)
	if !a.Check(r, Read) {
		t.Fatal("valid duplicate cookie should authorize")
	}
}
