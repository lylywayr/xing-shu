package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticHandlerDoesNotCacheHTMLShell(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>new-shell</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()
	staticHandler(http.FileServer(http.Dir(dir))).ServeHTTP(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "new-shell") {
		t.Fatalf("static shell not served: code=%d body=%q", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Cache-Control"); got != "no-store, no-cache, must-revalidate" {
		t.Fatalf("HTML shell must not be cached, got %q", got)
	}
}

func TestAdminAuthorizerUsesAPIKeyFallback(t *testing.T) {
	t.Setenv("ADMIN_API_KEY", "api-key")
	t.Setenv("XING_SHU_ADMIN_USER", "")
	t.Setenv("XING_SHU_ADMIN_PASSWORD", "")
	a := adminAuthorizer()
	if a.Username != "admin" || a.Password != "api-key" {
		t.Fatalf("unexpected fallback auth: user=%q password_set=%t", a.Username, a.Password != "")
	}
}
