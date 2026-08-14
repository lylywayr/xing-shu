package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditViewFiltersAndPaginates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	data := "{\"model\":\"alpha\",\"status\":200,\"latency_ms\":10}\n{\"model\":\"beta\",\"status\":500,\"latency_ms\":90}\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	AuditView(path).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v2/admin/recent?model=beta&status=500&limit=1", nil))
	var body struct {
		Requests []map[string]any `json:"requests"`
		Total    int              `json:"total"`
		Limit    int              `json:"limit"`
	}
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || len(body.Requests) != 1 || body.Total != 1 || body.Limit != 1 || body.Requests[0]["model"] != "beta" {
		t.Fatalf("unexpected audit result: %s", w.Body.String())
	}
}

func TestAlertActionPersistsState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alerts.jsonl")
	if err := os.WriteFile(path, []byte("{\"id\":\"a1\",\"level\":\"warning\",\"message\":\"x\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	AlertAction(path).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v2/admin/alerts/action", strings.NewReader(`{"id":"a1","action":"acknowledge","note":"handled"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
