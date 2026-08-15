package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"xing-shu/internal/catalog"
	"xing-shu/internal/governance"
	"xing-shu/internal/provider"
)

func TestOverviewViewAggregatesLiveState(t *testing.T) {
	dir := t.TempDir()
	audit := filepath.Join(dir, "audit.jsonl")
	alerts := filepath.Join(dir, "alerts.jsonl")
	if err := os.WriteFile(audit, []byte(`{"model":"m1","status":200}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(alerts, []byte(`{"level":"warning","message":"test"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	manager := catalog.NewManager(catalog.Catalog{Models: []catalog.Model{{ID: "m1", Provider: "p1", Status: catalog.Active, Admitted: true, AutoRoutable: true}}}, nil)
	ops := NewOps()
	ops.Disabled["p1"] = true
	runtime := ProviderRuntime{Configs: map[string]provider.Config{"p1": {ID: "p1", Kind: "credit"}}, Manager: manager, Ops: ops}
	handler := OverviewView(manager, runtime, nil, audit, alerts)
	request := httptest.NewRequest(http.MethodGet, "/api/admin/overview", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", response.Code)
	}
	var body struct {
		Health struct {
			Service map[string]any `json:"service"`
			Overall string         `json:"overall"`
		} `json:"health"`
		Consistency map[string]any   `json:"consistency"`
		Providers   []map[string]any `json:"providers"`
		Recent      struct {
			Count int `json:"count"`
		} `json:"recent"`
		Alerts struct {
			Count int `json:"count"`
		} `json:"alerts"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Health.Service["status"] != "degraded" || body.Health.Overall != "degraded" {
		t.Fatalf("unexpected health: %+v", body.Health)
	}
	if body.Consistency["ok"] != true {
		t.Fatalf("unexpected consistency: %+v", body.Consistency)
	}
	if len(body.Providers) != 1 || body.Providers[0]["status"] != "disabled" {
		t.Fatalf("unexpected providers: %+v", body.Providers)
	}
	if body.Recent.Count != 1 || body.Alerts.Count != 1 {
		t.Fatalf("unexpected summaries: recent=%d alerts=%d", body.Recent.Count, body.Alerts.Count)
	}
}

func TestOverviewContainsOnlyServiceHealth(t *testing.T) {
	manager := catalog.NewManager(catalog.Catalog{Models: []catalog.Model{{ID: "m1", Provider: "p1", Status: catalog.Active}}}, nil)
	handler := OverviewView(manager, ProviderRuntime{}, []governance.Record{}, filepath.Join(t.TempDir(), "missing-audit"), filepath.Join(t.TempDir(), "missing-alerts"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/admin/overview", nil))
	var body struct {
		Health struct {
			Service map[string]any `json:"service"`
		} `json:"health"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Health.Service["status"] != "ready" {
		t.Fatalf("unexpected service health: %+v", body.Health)
	}
}
