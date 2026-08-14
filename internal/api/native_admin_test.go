package api

import (
	"testing"
	"time"
	"xing-shu/internal/catalog"
	"xing-shu/internal/provider"
)

func TestProviderListIncludesOperationalStatus(t *testing.T) {
	ops := NewOps()
	ops.Disabled["p1"] = true
	runtime := ProviderRuntime{Configs: map[string]provider.Config{"p1": {ID: "p1", Kind: "credit"}}, Manager: catalog.NewManager(catalog.Catalog{}, nil), Ops: ops}
	items := runtime.List()
	if len(items) != 1 || items[0]["disabled"] != true || items[0]["status"] != "disabled" {
		t.Fatalf("missing provider status: %+v", items)
	}
}

func TestProviderListIncludesSyncAndCooldownFacts(t *testing.T) {
	manager := catalog.NewManager(catalog.Catalog{}, nil)
	ops := NewOps()
	until := time.Now().Add(10 * time.Minute).UTC()
	ops.CooldownUntil["p1"] = until
	runtime := ProviderRuntime{Configs: map[string]provider.Config{"p1": {ID: "p1", Kind: "credit"}}, Manager: manager, Ops: ops}
	manager.MarkSynced("p1", time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC))
	item := runtime.List()[0]
	if item["status"] != "cooldown" || item["cooldown_active"] != true {
		t.Fatalf("missing cooldown status: %+v", item)
	}
	if item["last_sync"] != "2026-08-09T00:00:00Z" {
		t.Fatalf("missing last sync: %+v", item)
	}
	if item["cooldown_until"] != until.Format(time.RFC3339) {
		t.Fatalf("missing cooldown deadline: %+v", item)
	}
}

func TestProviderListIncludesLastSyncError(t *testing.T) {
	ops := NewOps()
	ops.MarkSyncError("p1", "timeout", "provider did not respond")
	runtime := ProviderRuntime{Configs: map[string]provider.Config{"p1": {ID: "p1", Kind: "credit"}}, Manager: catalog.NewManager(catalog.Catalog{}, nil), Ops: ops}
	item := runtime.List()[0]
	if item["sync_error_type"] != "timeout" || item["sync_error"] != "provider did not respond" {
		t.Fatalf("missing sync error facts: %+v", item)
	}
}
