package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"xing-shu/internal/catalog"
	"xing-shu/internal/provider"
)

func TestReviewerConnectionClassifiesSuccessfulStructuredResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": `{"ok":true}`}}}})
	}))
	defer upstream.Close()
	manager := catalog.NewManager(catalog.Catalog{Models: []catalog.Model{{ID: "teacher", Provider: "p1", Status: catalog.Active, AutoRoutable: true, StructuredOutput: true, StructuredOutputKnown: true}}}, nil)
	runtime := &ReviewerConnectionRuntime{Configs: map[string]provider.Config{"p1": {ID: "p1", BaseURL: upstream.URL}}, Manager: manager}
	req := httptest.NewRequest(http.MethodPost, "/v2/admin/reviewer/test", strings.NewReader(`{"provider":"p1","model":"teacher"}`))
	w := httptest.NewRecorder()
	runtime.Test(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["network"] != true || body["http"] != true || body["json"] != true || body["structured_output"] != true {
		t.Fatalf("missing success facts: %+v", body)
	}
}

func TestReviewerConnectionRejectsUnknownModelAndInvalidJSON(t *testing.T) {
	manager := catalog.NewManager(catalog.Catalog{Models: []catalog.Model{{ID: "teacher", Provider: "p1", Status: catalog.Active}}}, nil)
	runtime := &ReviewerConnectionRuntime{Configs: map[string]provider.Config{"p1": {ID: "p1", BaseURL: "http://127.0.0.1"}}, Manager: manager}
	for _, body := range []string{`{"provider":"p1","model":"missing"}`, `not-json`} {
		req := httptest.NewRequest(http.MethodPost, "/v2/admin/reviewer/test", strings.NewReader(body))
		w := httptest.NewRecorder()
		runtime.Test(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	}
}

func TestReviewerConnectionClassifiesProviderAuthFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "forbidden", http.StatusForbidden) }))
	defer upstream.Close()
	manager := catalog.NewManager(catalog.Catalog{Models: []catalog.Model{{ID: "teacher", Provider: "p1", Status: catalog.Active}}}, nil)
	runtime := &ReviewerConnectionRuntime{Configs: map[string]provider.Config{"p1": {ID: "p1", BaseURL: upstream.URL}}, Manager: manager}
	req := httptest.NewRequest(http.MethodPost, "/v2/admin/reviewer/test", strings.NewReader(`{"provider":"p1","model":"teacher"}`))
	w := httptest.NewRecorder()
	runtime.Test(w, req)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != http.StatusOK || body["auth"] != false || body["error_type"] != "auth" {
		t.Fatalf("unexpected auth failure: %d %+v", w.Code, body)
	}
}
