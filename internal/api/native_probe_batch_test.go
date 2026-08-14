package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"xing-shu/internal/catalog"
	"xing-shu/internal/provider"
)

func TestProbeBatchRejectsUnknownOrDuplicateModels(t *testing.T) {
	manager := catalog.NewManager(catalog.Catalog{Models: []catalog.Model{{ID: "m1", Provider: "p1", Status: catalog.Active}}}, nil)
	runtime := &ProbeRuntime{Configs: map[string]provider.Config{"p1": {ID: "p1", BaseURL: "http://127.0.0.1"}}, Manager: manager}
	batch := NewProbeBatchRuntime(runtime, "")
	req := httptest.NewRequest(http.MethodPost, "/api/admin/probe/batch", strings.NewReader(`{"models":[{"provider":"p1","model":"m1"},{"provider":"p1","model":"m1"},{"provider":"p1","model":"missing"}]}`))
	w := httptest.NewRecorder()
	batch.Start(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid batch, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProbeBatchReportsProgressAndPersistsHistory(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": `{"ok":true}`}}}})
	}))
	defer upstream.Close()
	manager := catalog.NewManager(catalog.Catalog{Models: []catalog.Model{{ID: "m1", Provider: "p1", Status: catalog.Active}}}, nil)
	runtime := &ProbeRuntime{Configs: map[string]provider.Config{"p1": {ID: "p1", BaseURL: upstream.URL}}, Manager: manager}
	batch := NewProbeBatchRuntime(runtime, t.TempDir())
	req := httptest.NewRequest(http.MethodPost, "/api/admin/probe/batch", strings.NewReader(`{"models":[{"provider":"p1","model":"m1"}]}`))
	w := httptest.NewRecorder()
	batch.Start(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var started map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &started); err != nil || started["job_id"] == nil {
		t.Fatalf("missing job id: %s", w.Body.String())
	}
	var status map[string]any
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		status = batch.Get(started["job_id"].(string))
		if status["status"] == "completed" {
			break
		}
	}
	if status["status"] != "completed" || status["completed"] != float64(1) {
		t.Fatalf("unexpected batch status: %+v", status)
	}
	if len(batch.HistoryItems()) != 1 {
		t.Fatalf("expected one persisted history item, got %d", len(batch.HistoryItems()))
	}
}

func TestProbeBatchHistoryReturnsEmptyArray(t *testing.T) {
	batch := NewProbeBatchRuntime(nil, "")
	req := httptest.NewRequest(http.MethodGet, "/api/admin/probe/batch/history", nil)
	w := httptest.NewRecorder()
	batch.History(w, req)
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["items"] == nil {
		t.Fatalf("expected empty array, got %s", w.Body.String())
	}
}

func TestProbeBatchRejectsMoreThanFiftyTargets(t *testing.T) {
	manager := catalog.NewManager(catalog.Catalog{Models: []catalog.Model{{ID: "m1", Provider: "p1", Status: catalog.Active}}}, nil)
	runtime := &ProbeRuntime{Configs: map[string]provider.Config{"p1": {ID: "p1", BaseURL: "http://127.0.0.1"}}, Manager: manager}
	batch := NewProbeBatchRuntime(runtime, "")
	targets := make([]ProbeTarget, 51)
	for i := range targets {
		targets[i] = ProbeTarget{Provider: "p1", Model: "m1"}
	}
	body, _ := json.Marshal(ProbeBatchRequest{Models: targets})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/probe/batch", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	batch.Start(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized batch, got %d", w.Code)
	}
}
