package catalog

import "testing"

func TestRestoreReviewerCapabilityMarksSelectedActiveModel(t *testing.T) {
	manager := NewManager(Catalog{Models: []Model{{ID: "teacher", Provider: "p", Status: Active, AutoRoutable: true}}}, nil)
	if !manager.RestoreReviewerCapability("teacher", "p") {
		t.Fatal("selected model not found")
	}
	model := manager.Snapshot().Models[0]
	if !model.StructuredOutput || !model.StructuredOutputKnown {
		t.Fatalf("capability not restored: %+v", model)
	}
	if model.CapabilityEvidence["structured_output"].Source != "persisted reviewer selection" {
		t.Fatal("restore evidence missing")
	}
}
