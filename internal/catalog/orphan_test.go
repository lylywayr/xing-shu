package catalog

import (
	"testing"
	"xing-shu/internal/provider"
)

func TestReconcileProvidersMarksOrphansStaleAndRestoresUnknown(t *testing.T) {
	m := NewManager(Catalog{Models: []Model{
		{ID: "kept", Provider: "online", Status: Active, AutoRoutable: true},
		{ID: "orphan", Provider: "gone", Status: Active, AutoRoutable: true},
		{ID: "already-stale", Provider: "online", Status: Stale, AutoRoutable: false, Orphaned: true},
		{ID: "fresh-stale", Provider: "online", Status: Stale, AutoRoutable: false},
	}}, map[string]bool{"gone/orphan": true})
	m.ReconcileProviders(map[string]provider.Config{"online": {ID: "online", Enabled: true}})
	snapshot := m.Snapshot()
	states := map[string]Model{}
	for _, model := range snapshot.Models {
		states[model.ID] = model
	}
	if states["orphan"].Status != Stale || states["orphan"].AutoRoutable {
		t.Fatalf("orphan not isolated: %+v", states["orphan"])
	}
	if states["kept"].Status != Active || !states["kept"].AutoRoutable {
		t.Fatalf("online model changed: %+v", states["kept"])
	}
	if states["already-stale"].Status != Unknown {
		t.Fatalf("restored provider should return orphan-stale model to unknown: %+v", states["already-stale"])
	}
	if states["fresh-stale"].Status != Stale {
		t.Fatalf("ordinary stale model must remain stale: %+v", states["fresh-stale"])
	}
}
