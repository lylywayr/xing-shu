package api

import (
	"testing"
	"xing-shu/internal/catalog"
)

func TestModelsResponseExcludesInactiveAndMetaModels(t *testing.T) {
	response := ModelsResponse(catalog.Catalog{Models: []catalog.Model{{ID: "ok", Provider: "p", Status: catalog.Active, AutoRoutable: true}, {ID: "old", Provider: "p", Status: catalog.Stale, AutoRoutable: true}, {ID: "auto", Provider: "p", Status: catalog.Active, AutoRoutable: true}}})
	data := response["data"].([]catalog.Model)
	if len(data) != 2 || data[1].ID != "ok" {
		t.Fatalf("unexpected public model list: %+v", data)
	}
}
