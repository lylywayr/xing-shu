package catalog

import "testing"

func TestUpdateProviderModelDoesNotCrossProvider(t *testing.T) {
	manager := NewManager(Catalog{Models: []Model{{ID: "same", Provider: "p1"}, {ID: "same", Provider: "p2"}}}, nil)
	if !manager.UpdateProviderModel("same", "p2", func(model *Model) { model.StructuredOutput = true; model.StructuredOutputKnown = true }) {
		t.Fatal("expected provider model to update")
	}
	models := manager.Snapshot().Models
	if models[0].StructuredOutputKnown || !models[1].StructuredOutputKnown {
		t.Fatalf("updated wrong provider: %+v", models)
	}
}
