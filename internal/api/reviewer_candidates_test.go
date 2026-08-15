package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"xing-shu/internal/catalog"
	"xing-shu/internal/runtime"
)

func TestReviewerCandidatesExplainEligibility(t *testing.T) {
	manager := catalog.NewManager(catalog.Catalog{Models: []catalog.Model{
		{ID: "json-model", Provider: "p1", Status: catalog.Active, Admitted: true, AutoRoutable: true, StructuredOutput: true, StructuredOutputKnown: true},
		{ID: "unknown-capability", Provider: "p1", Status: catalog.Active, Admitted: true, AutoRoutable: true},
		{ID: "not-active", Provider: "p1", Status: catalog.Unknown, AutoRoutable: true, StructuredOutput: true, StructuredOutputKnown: true},
	}}, map[string]bool{})
	rt := &Runtime{ReviewerSelection: runtime.NewReviewerSelection()}
	h := ReviewerCandidatesView(manager, nil, rt)
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest("GET", "/api/admin/reviewer/candidates", nil))
	if w.Code != 200 {
		t.Fatalf("want 200 got %d", w.Code)
	}
	var got struct {
		Items    []map[string]any `json:"items"`
		Eligible int              `json:"eligible"`
		Excluded int              `json:"excluded"`
		Reasons  map[string]int   `json:"reasons"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Eligible != 1 || len(got.Items) != 1 || got.Excluded != 2 {
		t.Fatalf("unexpected eligibility: %+v", got)
	}
	if got.Reasons["structured_output_unknown"] != 1 || got.Reasons["not_active"] != 1 {
		t.Fatalf("missing exclusion reasons: %+v", got.Reasons)
	}
}
