package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"xing-shu/internal/catalog"
	"xing-shu/internal/runtime"
)

func TestShadowBatchRequiresExplicitCandidateModel(t *testing.T) {
	rt := NewRuntimeAt(t.TempDir())
	rt.Knowledge.Upsert(runtime.Knowledge{ID: "k1", RecommendedModel: "m1", Status: runtime.KnowledgeTrusted})
	rt.Reviews.Add(runtime.Review{ID: "r1", Replayable: true, TaskPackage: []byte(`{"model":"auto"}`)})
	manager := catalog.NewManager(catalog.Catalog{Models: []catalog.Model{{ID: "m1", Provider: "p1", Status: catalog.Active, AutoRoutable: true}}}, nil)
	w := httptest.NewRecorder()
	ShadowBatchStart(rt, manager, NewOps(), runtime.NewShadowBatches()).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v2/admin/shadow/batches/start", strings.NewReader(`{"knowledge_id":"k1","review_ids":["r1"]}`)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
