package quota

import "testing"

func TestHasFactsOnlyForVerifiedStates(t *testing.T) {
	if HasFacts([]Snapshot{{Provider: "p", State: Unknown}}) {
		t.Fatal("unknown quota must remain unavailable")
	}
	if !HasFacts([]Snapshot{{Provider: "p", State: Available}}) {
		t.Fatal("available quota fact missing")
	}
	if !HasFacts([]Snapshot{{Provider: "p", State: Exhausted}}) {
		t.Fatal("exhausted quota is still a verified fact")
	}
}
