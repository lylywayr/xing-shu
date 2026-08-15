package api

import (
	"testing"
	"xing-shu/internal/catalog"
)

func TestModelsResponseRequiresAdmissionAndAutoApproval(t *testing.T) {
	response := ModelsResponse(catalog.Catalog{Models: []catalog.Model{{ID: "not-admitted", Provider: "p", Status: catalog.Active, AutoRoutable: true}, {ID: "approved", Provider: "p", Status: catalog.Active, Admitted: true, AutoRoutable: true}}})
	data := response["data"].([]catalog.Model)
	if len(data) != 2 || data[1].ID != "approved" {
		t.Fatalf("public list bypassed admission: %+v", data)
	}
}

func TestModelsResponseExcludesInactiveAndMetaModels(t *testing.T) {
	response := ModelsResponse(catalog.Catalog{Models: []catalog.Model{{ID: "ok", Provider: "p", Status: catalog.Active, Admitted: true, AutoRoutable: true}, {ID: "old", Provider: "p", Status: catalog.Stale, AutoRoutable: true}, {ID: "auto", Provider: "p", Status: catalog.Active, Admitted: true, AutoRoutable: true}}})
	data := response["data"].([]catalog.Model)
	if len(data) != 2 || data[1].ID != "ok" {
		t.Fatalf("unexpected public model list: %+v", data)
	}
}
