package catalog

import (
	"testing"
	"xing-shu/internal/provider"
)

func TestMeta(t *testing.T) {
	if !IsMeta("compound") {
		t.Fatal("meta not detected")
	}
}
func TestAdmissionAndAutoApprovalApply(t *testing.T) {
	x := SyncService{Allow: map[string]bool{"p/good": true}, Admitted: map[string]bool{"p/good": true}}.Apply("p", []provider.RawModel{{ID: "good", StructuredOutput: true}, {ID: "new"}})
	if !x[0].Admitted || !x[0].AutoRoutable || !x[0].StructuredOutput || x[1].Admitted || x[1].AutoRoutable {
		t.Fatal("admission or auto approval propagation failed")
	}
}
