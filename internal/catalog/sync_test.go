package catalog

import (
	"testing"
	"xing-shu/internal/provider"
)

func TestSyncServiceFiltersMetaAndInvalidModels(t *testing.T) {
	items := (SyncService{Allow: map[string]bool{"p/good": true}}).Apply("p", []provider.RawModel{{ID: "good", Context: 4096}, {ID: "auto"}, {ID: "fusion"}, {ID: "compound"}, {ID: "bad\nmodel"}, {ID: "huge", Context: 999999999}})
	if len(items) != 2 {
		t.Fatalf("unexpected cleaned catalog: %+v", items)
	}
	if items[0].ID != "good" || items[1].ContextWindow != 0 {
		t.Fatalf("invalid model data survived: %+v", items)
	}
}
