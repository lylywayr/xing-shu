package catalog

import (
	"testing"
	"xing-shu/internal/provider"
)

func TestSyncServiceRequiresApprovalOutsideFreeLLMAPI(t *testing.T) {
	for _, providerID := range []string{"workbuddy-141de3", "trae-de85c7", "qoder-9d2fb6"} {
		items := (SyncService{Allow: map[string]bool{}}).Apply(providerID, []provider.RawModel{{ID: "new-model"}})
		if len(items) != 1 || items[0].Status != Unknown || items[0].AutoRoutable {
			t.Fatalf("%s model must wait for explicit approval: %+v", providerID, items)
		}
	}
}

func TestSyncServiceAutoApprovesFreeLLMAPI(t *testing.T) {
	items := (SyncService{Allow: map[string]bool{}}).Apply(FreeLLMAPIProviderID, []provider.RawModel{{ID: "free-model"}})
	if len(items) != 1 || items[0].Status != Active || !items[0].AutoRoutable {
		t.Fatalf("FreeLLMAPI model should be auto-approved: %+v", items)
	}
}

func TestSyncServiceFiltersMetaAndInvalidModels(t *testing.T) {
	items := (SyncService{Allow: map[string]bool{"p/good": true}}).Apply("p", []provider.RawModel{{ID: "good", Context: 4096}, {ID: "auto"}, {ID: "fusion"}, {ID: "compound"}, {ID: "bad\nmodel"}, {ID: "huge", Context: 999999999}})
	if len(items) != 2 {
		t.Fatalf("unexpected cleaned catalog: %+v", items)
	}
	if items[0].ID != "good" || items[1].ContextWindow != 0 {
		t.Fatalf("invalid model data survived: %+v", items)
	}
}
