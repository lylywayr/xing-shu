package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"xing-shu/internal/catalog"
	"xing-shu/internal/routing"
	"xing-shu/internal/runtime"
)

func TestRoutingExplainReportsRequestAndScoreBreakdown(t *testing.T) {
	manager := catalog.NewManager(catalog.Catalog{Models: []catalog.Model{
		{ID: "tools-a", Provider: "p1", Status: catalog.Active, Admitted: true, AutoRoutable: true, Tools: true, Score: 80, Capabilities: []string{"tools"}},
		{ID: "text-b", Provider: "p1", Status: catalog.Active, Admitted: true, AutoRoutable: true, Score: 70},
		{ID: "disabled-c", Provider: "p2", Status: catalog.Disabled, AutoRoutable: false, Score: 90},
	}}, nil)
	service := &routing.Service{Providers: map[string]routing.ProviderConfig{"p1": {ID: "p1", BaseURL: "http://p1"}, "p2": {ID: "p2", BaseURL: "http://p2"}}, Manager: manager, Knowledge: runtime.NewKnowledgeStore()}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/routing/explain", strings.NewReader(`{"model":"auto","messages":[{"role":"user","content":"use a tool"}],"tools":[{"type":"function","function":{"name":"lookup"}}]}`))
	w := httptest.NewRecorder()
	RoutingExplain(service).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["need"] != "tools" || body["selected"] != "tools-a" {
		t.Fatalf("unexpected decision: %+v", body)
	}
	items, ok := body["candidates"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("missing candidate explanations: %+v", body)
	}
	first := items[0].(map[string]any)
	if first["score_breakdown"] == nil || first["eligible"] != true {
		t.Fatalf("missing score breakdown: %+v", first)
	}
}

func TestRoutingExplainRejectsInvalidJSON(t *testing.T) {
	service := &routing.Service{}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/routing/explain", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	RoutingExplain(service).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRoutingExplainKeepsGETItemsContract(t *testing.T) {
	manager := catalog.NewManager(catalog.Catalog{Models: []catalog.Model{{ID: "m1", Provider: "p1", Status: catalog.Active, Admitted: true, AutoRoutable: true}}}, nil)
	service := &routing.Service{Providers: map[string]routing.ProviderConfig{"p1": {ID: "p1", BaseURL: "http://p1"}}, Manager: manager}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/routing/explain", nil)
	w := httptest.NewRecorder()
	RoutingExplain(service).ServeHTTP(w, req)
	var body map[string]any
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &body) != nil || body["items"] == nil {
		t.Fatalf("GET contract failed: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestRoutingExplainMatchesSelectAutoKnowledgeBonus(t *testing.T) {
	expires := time.Now().Add(30 * 24 * time.Hour)
	knowledge := runtime.NewKnowledgeStore()
	knowledge.Upsert(runtime.Knowledge{ID: "trusted-tools", Signature: runtime.Signature(runtime.TaskFeatures{NeedsTools: true}), RecommendedModel: "m1", Status: runtime.KnowledgeTrusted, Confidence: 1, ValidatedSamples: 10, SuccessfulSamples: 10, ExpiresAt: &expires})
	manager := catalog.NewManager(catalog.Catalog{Models: []catalog.Model{
		{ID: "m1", Provider: "p1", Status: catalog.Active, Admitted: true, AutoRoutable: true, Tools: true, Capabilities: []string{"tools"}, Score: 10},
		{ID: "m2", Provider: "p1", Status: catalog.Active, Admitted: true, AutoRoutable: true, Tools: true, Capabilities: []string{"tools"}, Score: 50},
	}}, nil)
	service := &routing.Service{Providers: map[string]routing.ProviderConfig{"p1": {ID: "p1", BaseURL: "http://p1"}}, Manager: manager, Knowledge: knowledge}
	body := []byte(`{"model":"auto","messages":[{"role":"user","content":"use tools"}],"tools":[{"type":"function","function":{"name":"lookup"}}]}`)
	if selected := service.SelectAuto(body); selected != "m1" {
		t.Fatalf("expected knowledge-boosted m1, got %s", selected)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/routing/explain", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	RoutingExplain(service).ServeHTTP(w, req)
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["selected"] != "m1" {
		t.Fatalf("explain selected a different model: %+v", response)
	}
	for _, raw := range response["candidates"].([]any) {
		candidate := raw.(map[string]any)
		if candidate["model"] == "m1" {
			breakdown := candidate["score_breakdown"].(map[string]any)
			if breakdown["knowledge_bonus"].(float64) <= 0 {
				t.Fatalf("missing knowledge bonus: %+v", candidate)
			}
			return
		}
	}
	t.Fatal("m1 candidate missing")
}
