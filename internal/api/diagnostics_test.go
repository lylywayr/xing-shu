package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeDiagnosticsReturnsFileMetrics(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "reviews-xing-shu.json"), []byte(`{"r":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	RuntimeDiagnostics(dir).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/admin/diagnostics", nil))
	var body struct {
		Items []RuntimeFileMetric `json:"items"`
	}
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || len(body.Items) == 0 {
		t.Fatalf("invalid diagnostics: %s", w.Body.String())
	}
}

func TestWatchdogStatusReadsFailureState(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "failures"), []byte("3"), 0600); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	WatchdogStatusView(dir).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/admin/watchdog", nil))
	var body WatchdogStatus
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || body.ConsecutiveFailures != 3 || body.Healthy {
		t.Fatalf("invalid watchdog state: %s", w.Body.String())
	}
}
