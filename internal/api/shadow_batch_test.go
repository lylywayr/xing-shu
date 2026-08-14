package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"xing-shu/internal/runtime"
)

func TestShadowBatchesViewFiltersByStatus(t *testing.T) {
	batches := runtime.NewShadowBatches()
	batches.Put(runtime.ShadowBatch{ID: "b1", Status: runtime.ShadowBatchCompleted})
	batches.Put(runtime.ShadowBatch{ID: "b2", Status: runtime.ShadowBatchRunning})
	w := httptest.NewRecorder()
	ShadowBatchesView(batches).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/admin/shadow/batches?status=running", nil))
	var body struct {
		Items []runtime.ShadowBatch `json:"items"`
	}
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || len(body.Items) != 1 || body.Items[0].ID != "b2" {
		t.Fatalf("unexpected filtered batches: %s", w.Body.String())
	}
}

func TestShadowBatchStopRequiresBatchID(t *testing.T) {
	batches := runtime.NewShadowBatches()
	w := httptest.NewRecorder()
	ShadowBatchStop(batches).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/admin/shadow/batches/stop", strings.NewReader(`{}`)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
