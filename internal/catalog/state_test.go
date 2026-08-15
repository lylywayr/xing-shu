package catalog

import (
	"path/filepath"
	"testing"
)

func TestStateRoundTripAndManagerPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog-xing-shu.json")
	original := State{Catalog: Catalog{Version: "r1", Models: []Model{{ID: "m", Provider: "p", Status: Active}}}, Allow: map[string]bool{"p/m": true}}
	if err := SaveState(path, original); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Catalog.Version != "r1" || len(got.Catalog.Models) != 1 || !got.Allow["p/m"] {
		t.Fatalf("state was not preserved: %+v", got)
	}
	manager := NewManager(got.Catalog, got.Allow)
	var persisted State
	manager.SetPersist(func(state State) { persisted = state })
	if !manager.SetAllow("p", "m", false) || persisted.Allow["p/m"] {
		t.Fatalf("manager change was not persisted: %+v", persisted)
	}
}

func TestSetAllowActivatesUnknownModel(t *testing.T) {
	manager := NewManager(Catalog{Models: []Model{{ID: "m", Provider: "p", Status: Unknown}}}, nil)
	if !manager.SetAllow("p", "m", true) {
		t.Fatal("expected model approval to change the catalog")
	}
	model := manager.Snapshot().Models[0]
	if model.Status != Active || !model.AutoRoutable {
		t.Fatalf("approved model did not become routable: %+v", model)
	}
	if !manager.SetAllow("p", "m", false) {
		t.Fatal("expected model rejection to change the catalog")
	}
	model = manager.Snapshot().Models[0]
	if model.Status != Unknown || model.AutoRoutable {
		t.Fatalf("rejected model did not return to pending: %+v", model)
	}
}

func TestApplyDefaultApprovalPolicyMigratesOnlyFreeLLMAPI(t *testing.T) {
	manager := NewManager(Catalog{Models: []Model{
		{ID: "free", Provider: FreeLLMAPIProviderID, Status: Unknown},
		{ID: "stale", Provider: FreeLLMAPIProviderID, Status: Stale},
		{ID: "external", Provider: "workbuddy", Status: Unknown},
	}}, nil)
	if got := manager.ApplyDefaultApprovalPolicy(); got != 1 {
		t.Fatalf("expected one migrated model, got %d", got)
	}
	models := manager.Snapshot().Models
	if models[0].Status != Active || !models[0].AutoRoutable {
		t.Fatalf("FreeLLMAPI model was not migrated: %+v", models[0])
	}
	if models[1].Status != Stale || models[1].AutoRoutable || models[2].Status != Unknown || models[2].AutoRoutable {
		t.Fatalf("migration changed protected models: %+v", models)
	}
	if got := manager.ApplyDefaultApprovalPolicy(); got != 0 {
		t.Fatalf("migration must be idempotent, got %d changes", got)
	}
}

func TestLoadStateMissingFileIsEmpty(t *testing.T) {
	state, err := LoadState(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || len(state.Catalog.Models) != 0 || state.Allow == nil {
		t.Fatalf("missing state must be empty and valid: %+v %v", state, err)
	}
}
