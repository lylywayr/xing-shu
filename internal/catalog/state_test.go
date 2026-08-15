package catalog

import (
	"path/filepath"
	"testing"
	"time"
)

func verifiedModel(id, providerID string) Model {
	now := time.Now().UTC()
	return Model{ID: id, Provider: providerID, Status: Active, Admitted: true, CapabilityEvidence: map[string]Evidence{
		"structured_output": {Supported: true, Source: "runtime_probe", Level: "protocol", CheckedAt: now}, "tools": {Supported: false, Source: "runtime_probe", Level: "protocol", CheckedAt: now}, "vision": {Supported: false, Source: "runtime_probe", Level: "protocol", CheckedAt: now},
	}}
}
func TestStateRoundTripAndManagerPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog-xing-shu.json")
	original := State{Catalog: Catalog{Version: "r1", Models: []Model{{ID: "m", Provider: "p", Status: Active, Admitted: true}}}, Allow: map[string]bool{"p/m": true}, Admitted: map[string]bool{"p/m": true}}
	if err := SaveState(path, original); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(path)
	if err != nil || got.Catalog.Version != "r1" || !got.Allow["p/m"] || !got.Admitted["p/m"] {
		t.Fatalf("state was not preserved: %+v %v", got, err)
	}
	manager := NewManagerWithState(got)
	var persisted State
	manager.SetPersist(func(state State) { persisted = state })
	if !manager.SetAllow("p", "m", false) || persisted.Allow["p/m"] {
		t.Fatalf("Auto approval was not persisted: %+v", persisted)
	}
}
func TestAdmissionThenVerifiedAutoApproval(t *testing.T) {
	manager := NewManager(Catalog{Models: []Model{{ID: "m", Provider: "p", Status: Unknown}}}, nil)
	if !manager.SetAdmitted("p", "m", true) {
		t.Fatal("expected admission")
	}
	model := manager.Snapshot().Models[0]
	if model.Status != Active || !model.Admitted || model.AutoRoutable || manager.SetAllow("p", "m", true) {
		t.Fatalf("unverified model bypassed second gate: %+v", model)
	}
	manager.UpdateProviderModel("m", "p", func(x *Model) { *x = verifiedModel("m", "p") })
	if !manager.SetAllow("p", "m", true) {
		t.Fatal("expected verified Auto approval")
	}
	if !manager.Snapshot().Models[0].AutoRoutable {
		t.Fatal("verified approval not routable")
	}
	if !manager.SetAdmitted("p", "m", false) {
		t.Fatal("expected admission revoke")
	}
	model = manager.Snapshot().Models[0]
	if model.Status != Unknown || model.AutoRoutable || model.Admitted {
		t.Fatalf("revoked model not isolated: %+v", model)
	}
}
func TestApplyDefaultApprovalPolicyMigratesOnlyFreeLLMAPI(t *testing.T) {
	manager := NewManager(Catalog{Models: []Model{{ID: "free", Provider: FreeLLMAPIProviderID, Status: Unknown}, {ID: "stale", Provider: FreeLLMAPIProviderID, Status: Stale}, {ID: "external", Provider: "workbuddy", Status: Unknown}}}, nil)
	if got := manager.ApplyDefaultApprovalPolicy(); got != 1 {
		t.Fatalf("expected one migrated model, got %d", got)
	}
	models := manager.Snapshot().Models
	if models[0].Status != Active || !models[0].AutoRoutable || !models[0].Admitted {
		t.Fatalf("FreeLLMAPI model was not migrated: %+v", models[0])
	}
	if models[1].Status != Stale || models[1].AutoRoutable || models[2].Status != Unknown || models[2].AutoRoutable {
		t.Fatalf("migration changed protected models: %+v", models)
	}
}
func TestLoadStateMissingFileIsEmpty(t *testing.T) {
	state, err := LoadState(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || len(state.Catalog.Models) != 0 || state.Allow == nil || state.Admitted == nil {
		t.Fatalf("missing state must be empty and valid: %+v %v", state, err)
	}
}
