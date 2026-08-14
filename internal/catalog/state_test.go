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

func TestLoadStateMissingFileIsEmpty(t *testing.T) {
	state, err := LoadState(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || len(state.Catalog.Models) != 0 || state.Allow == nil {
		t.Fatalf("missing state must be empty and valid: %+v %v", state, err)
	}
}
