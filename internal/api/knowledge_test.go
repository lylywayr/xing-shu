package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"xing-shu/internal/runtime"
)

func TestKnowledgeDecisionRejectsInvalidTransition(t *testing.T) {
	store := runtime.NewKnowledgeStore()
	store.Upsert(runtime.Knowledge{ID: "k1", Status: runtime.KnowledgeProposed})
	rt := &Runtime{Knowledge: store}
	w := httptest.NewRecorder()
	KnowledgeDecision(rt).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v2/admin/knowledge/decision", strings.NewReader(`{"id":"k1","status":"trusted"}`)))
	if w.Code != http.StatusConflict {
		t.Fatalf("expected transition conflict, got %d: %s", w.Code, w.Body.String())
	}
}

func TestKnowledgeRestoreRequiresExpiredOrRejected(t *testing.T) {
	store := runtime.NewKnowledgeStore()
	store.Upsert(runtime.Knowledge{ID: "k1", Status: runtime.KnowledgeProposed})
	rt := &Runtime{Knowledge: store}
	w := httptest.NewRecorder()
	KnowledgeRestore(rt).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v2/admin/knowledge/restore", strings.NewReader(`{"id":"k1","status":"validated"}`)))
	if w.Code != http.StatusConflict {
		t.Fatalf("expected restore conflict, got %d: %s", w.Code, w.Body.String())
	}
}
