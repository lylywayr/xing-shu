package api

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"xing-shu/internal/catalog"
	"xing-shu/internal/governance"
)

func TestGovernanceViewReconcilesCatalogModels(t *testing.T) {
	models := catalog.Catalog{Models: []catalog.Model{{ID: "known", Provider: "p", Status: catalog.Active}, {ID: "new", Provider: "p", Status: catalog.Active}}}
	got := reconcileGovernance(models, []governance.Record{{Key: "p/known", Model: models.Models[0]}})
	if len(got) != 2 {
		t.Fatalf("governance count mismatch: %+v", got)
	}
	found := false
	for _, record := range got {
		if record.Key == "p/new" && record.Status == catalog.Unknown {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing reconciled record: %+v", got)
	}
}

func TestAlertsViewReturnsJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alerts.jsonl")
	if err := os.WriteFile(path, []byte("{\"level\":\"warning\",\"message\":\"x\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	AlertsView(path).ServeHTTP(w, httptest.NewRequest("GET", "/api/admin/alerts", nil))
	var got struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0]["message"] != "x" {
		t.Fatalf("unexpected alerts: %+v", got)
	}
}
