package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"xing-shu/internal/catalog"
	"xing-shu/internal/governance"
)

func TestSnapshotDetailReturnsMetadataAndDiff(t *testing.T) {
	manager := catalog.NewManager(catalog.Catalog{Models: []catalog.Model{{ID: "m1", Provider: "p", Status: catalog.Active}}}, nil)
	snap := governance.NewManager(t.TempDir())
	if err := snap.SaveCatalog(manager.Snapshot()); err != nil {
		t.Fatal(err)
	}
	name := snap.List()[0]
	rt := GovernanceRuntime{Snapshots: snap, Catalog: manager}
	w := httptest.NewRecorder()
	rt.Detail(w, httptest.NewRequest(http.MethodGet, "/v2/admin/snapshots/detail?name="+name, nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "sha256") {
		t.Fatalf("missing snapshot metadata: %d %s", w.Code, w.Body.String())
	}
}
