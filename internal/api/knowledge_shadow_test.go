package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"xing-shu/internal/catalog"
	"xing-shu/internal/runtime"
)

func TestShadowRunRequiresTrustedKnowledge(t *testing.T) {
	rt := NewRuntimeAt(t.TempDir())
	rt.Knowledge.Upsert(runtime.Knowledge{ID: "k1", RecommendedModel: "m1", Status: runtime.KnowledgeProposed})
	rt.Reviews.Add(runtime.Review{ID: "r1", Replayable: true, TaskPackage: []byte(`{"model":"auto"}`)})
	manager := catalog.NewManager(catalog.Catalog{Models: []catalog.Model{{ID: "m1", Provider: "p1", Status: catalog.Active, AutoRoutable: true}}}, nil)
	handler := ShadowRun(rt, manager, NewOps())
	req := httptest.NewRequest(http.MethodPost, "/v2/admin/shadow/run", strings.NewReader(`{"knowledge_id":"k1","review_id":"r1","candidate_model":"m1"}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestKnowledgeViewSupportsStatusFilter(t *testing.T) {
	store := runtime.NewKnowledgeStore()
	store.Upsert(runtime.Knowledge{ID: "proposed", Status: runtime.KnowledgeProposed})
	store.Upsert(runtime.Knowledge{ID: "rejected", Status: runtime.KnowledgeRejected})
	req := httptest.NewRequest(http.MethodGet, "/v2/admin/knowledge?status=proposed", nil)
	w := httptest.NewRecorder()
	KnowledgeView(store).ServeHTTP(w, req)
	var body struct {
		Items []runtime.Knowledge `json:"items"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &body) != nil || len(body.Items) != 1 || body.Items[0].Status != runtime.KnowledgeProposed {
		t.Fatalf("filter failed: %d %s", w.Code, w.Body.String())
	}
}

func TestKnowledgeViewIncludesLifecycleSummary(t *testing.T) {
	store := runtime.NewKnowledgeStore()
	store.Upsert(runtime.Knowledge{ID: "k1", Status: runtime.KnowledgeTrusted, ValidationHistory: []runtime.KnowledgeValidation{{ID: "v1"}}})
	w := httptest.NewRecorder()
	KnowledgeView(store).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v2/admin/knowledge", nil))
	var body struct {
		Items []runtime.Knowledge `json:"items"`
		Stats map[string]int      `json:"stats"`
	}
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || len(body.Items) != 1 || body.Items[0].ValidationHistory == nil || body.Stats["trusted"] != 1 {
		t.Fatalf("missing lifecycle fields: %s", w.Body.String())
	}
}
