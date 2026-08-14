package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"xing-shu/internal/auth"
	"xing-shu/internal/catalog"
)

func ruleManager() *catalog.Manager {
	return catalog.NewManager(catalog.Catalog{Models: []catalog.Model{
		{ID: "m1", Provider: "p1", Status: catalog.Unknown},
		{ID: "m2", Provider: "p1", Status: catalog.Active, AutoRoutable: true},
	}}, map[string]bool{"p1/m2": true})
}

func TestGovernanceRulesPreviewRejectsUnknownAndEmptyChanges(t *testing.T) {
	rules := NewGovernanceRulesRuntime(ruleManager(), t.TempDir())
	for _, body := range []string{`{"changes":[]}`, `{"changes":[{"key":"p1/missing","allow":true}]}`} {
		req := httptest.NewRequest(http.MethodPost, "/v2/admin/governance/rules/preview", strings.NewReader(body))
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
	req := httptest.NewRequest(http.MethodPost, "/v2/admin/governance/rules/apply", strings.NewReader(`{"changes":[{"key":"p1/m2","allow":false}]}`))
	w := httptest.NewRecorder()
	rules.Apply(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("apply failed: %d %s", w.Code, w.Body.String())
	}
	var applied map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &applied)
	if applied["revision"] == "" || manager.Snapshot().Models[1].AutoRoutable {
		t.Fatalf("apply did not update V2 rule: %+v", applied)
	}
	undo := httptest.NewRequest(http.MethodPost, "/v2/admin/governance/rules/undo", strings.NewReader(`{"revision":"`+applied["revision"].(string)+`"}`))
	uw := httptest.NewRecorder()
	rules.Undo(uw, undo)
	if uw.Code != http.StatusOK || !manager.Snapshot().Models[1].AutoRoutable {
		t.Fatalf("undo failed: %d %s", uw.Code, uw.Body.String())
	}
	if len(rules.Audit()) != 2 {
		t.Fatalf("expected apply and undo audit entries, got %d", len(rules.Audit()))
	}
}

func TestGovernanceRulesProtectedByOperatePermission(t *testing.T) {
	rules := NewGovernanceRulesRuntime(ruleManager(), t.TempDir())
	s := &Server{Auth: auth.Authorizer{AdminKey: "secret"}, GovernanceRules: rules}
	req := httptest.NewRequest(http.MethodPost, "/v2/admin/governance/rules/apply", strings.NewReader(`{"changes":[{"key":"p1/m2","allow":true}]}`))
	w := httptest.NewRecorder()
	s.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
