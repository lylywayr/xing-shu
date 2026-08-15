package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"xing-shu/internal/auth"
	"xing-shu/internal/catalog"
)

func readyForAuto(id, providerID string) catalog.Model {
	now := time.Now().UTC()
	return catalog.Model{ID: id, Provider: providerID, Status: catalog.Active, Admitted: true, CapabilityEvidence: map[string]catalog.Evidence{
		"structured_output": {Supported: true, Source: "runtime_probe", Level: "protocol", CheckedAt: now}, "tools": {Supported: false, Source: "runtime_probe", Level: "protocol", CheckedAt: now}, "vision": {Supported: false, Source: "runtime_probe", Level: "protocol", CheckedAt: now},
	}}
}
func ruleManager() *catalog.Manager {
	return catalog.NewManagerWithState(catalog.State{Catalog: catalog.Catalog{Models: []catalog.Model{{ID: "m1", Provider: "p1", Status: catalog.Unknown}, readyForAuto("m2", "p1")}}, Admitted: map[string]bool{"p1/m2": true}})
}
func TestGovernanceRulesPreviewRejectsUnverifiedAndEmptyChanges(t *testing.T) {
	rules := NewGovernanceRulesRuntime(ruleManager(), t.TempDir())
	for _, body := range []string{`{"changes":[]}`, `{"changes":[{"key":"p1/missing","allow":true}]}`, `{"changes":[{"key":"p1/m1","allow":true}]}`} {
		req := httptest.NewRequest(http.MethodPost, "/api/admin/governance/rules/preview", strings.NewReader(body))
		w := httptest.NewRecorder()
		rules.Preview(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	}
}
func TestGovernanceRulesApplyPersistsAuditAndUndoRestores(t *testing.T) {
	manager := ruleManager()
	rules := NewGovernanceRulesRuntime(manager, t.TempDir())
	allow := httptest.NewRequest(http.MethodPost, "/api/admin/governance/rules/apply", strings.NewReader(`{"changes":[{"key":"p1/m2","allow":true}]}`))
	aw := httptest.NewRecorder()
	rules.Apply(aw, allow)
	if aw.Code != http.StatusOK || !manager.Snapshot().Models[1].AutoRoutable {
		t.Fatalf("allow failed: %d %s", aw.Code, aw.Body.String())
	}
	deny := httptest.NewRequest(http.MethodPost, "/api/admin/governance/rules/apply", strings.NewReader(`{"changes":[{"key":"p1/m2","allow":false}]}`))
	dw := httptest.NewRecorder()
	rules.Apply(dw, deny)
	var applied map[string]any
	_ = json.Unmarshal(dw.Body.Bytes(), &applied)
	if dw.Code != http.StatusOK || manager.Snapshot().Models[1].AutoRoutable {
		t.Fatalf("deny failed: %d %s", dw.Code, dw.Body.String())
	}
	undo := httptest.NewRequest(http.MethodPost, "/api/admin/governance/rules/undo", strings.NewReader(`{"revision":"`+applied["revision"].(string)+`"}`))
	uw := httptest.NewRecorder()
	rules.Undo(uw, undo)
	if uw.Code != http.StatusOK || !manager.Snapshot().Models[1].AutoRoutable {
		t.Fatalf("undo failed: %d %s", uw.Code, uw.Body.String())
	}
}
func TestGovernanceRulesProtectedByOperatePermission(t *testing.T) {
	rules := NewGovernanceRulesRuntime(ruleManager(), t.TempDir())
	s := &Server{Auth: auth.Authorizer{AdminKey: "secret"}, GovernanceRules: rules}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/governance/rules/apply", strings.NewReader(`{"changes":[{"key":"p1/m2","allow":true}]}`))
	w := httptest.NewRecorder()
	s.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
