package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"xing-shu/internal/catalog"
)

func admissionManager() *catalog.Manager {
	return catalog.NewManager(catalog.Catalog{Models: []catalog.Model{{ID: "m", Provider: "p", Status: catalog.Unknown}}}, nil)
}
func TestModelAdmissionsApplyPersistsAndIsolatesAuto(t *testing.T) {
	manager := admissionManager()
	runtime := NewModelAdmissionRuntime(manager, t.TempDir())
	req := httptest.NewRequest(http.MethodPost, "/api/admin/model-admissions/apply", strings.NewReader(`{"changes":[{"key":"p/m","admitted":true}]}`))
	w := httptest.NewRecorder()
	runtime.Apply(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("apply failed: %d %s", w.Code, w.Body.String())
	}
	model := manager.Snapshot().Models[0]
	if !model.Admitted || model.Status != catalog.Active || model.AutoRoutable {
		t.Fatalf("admission did not preserve second gate: %+v", model)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["revision"] == "" || len(runtime.audits) != 1 {
		t.Fatalf("audit missing: %+v", body)
	}
	undo := httptest.NewRequest(http.MethodPost, "/api/admin/model-admissions/undo", strings.NewReader(`{"revision":"`+body["revision"].(string)+`"}`))
	uw := httptest.NewRecorder()
	runtime.Undo(uw, undo)
	if uw.Code != http.StatusOK || manager.Snapshot().Models[0].Admitted {
		t.Fatalf("undo did not isolate model: %d %s", uw.Code, uw.Body.String())
	}
}
func TestModelAdmissionsRejectFreeLLMAPIAndMissing(t *testing.T) {
	manager := catalog.NewManager(catalog.Catalog{Models: []catalog.Model{{ID: "f", Provider: catalog.FreeLLMAPIProviderID, Status: catalog.Active}}}, nil)
	runtime := NewModelAdmissionRuntime(manager, "")
	for _, key := range []string{"freellmapi/f", "p/missing"} {
		req := httptest.NewRequest(http.MethodPost, "/api/admin/model-admissions/preview", strings.NewReader(`{"changes":[{"key":"`+key+`","admitted":true}]}`))
		w := httptest.NewRecorder()
		runtime.Preview(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected rejection for %s, got %d", key, w.Code)
		}
	}
}
