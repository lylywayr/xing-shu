package quota

import (
	"testing"
	"time"
)

func TestProviderQuotaPoolsRemainIndependent(t *testing.T) {
	m := NewManager()
	a, b := 5.0, 9.0
	m.Set(Snapshot{Provider: "freellmapi", Pool: "a", Metric: "requests", State: Available, Remaining: &a, CheckedAt: time.Now()})
	m.Set(Snapshot{Provider: "freellmapi", Pool: "b", Metric: "tokens", State: Available, Remaining: &b, CheckedAt: time.Now().Add(time.Second)})
	if len(m.All()) != 2 {
		t.Fatalf("quota pools collapsed: %+v", m.All())
	}
	if got, ok := m.Get("freellmapi"); !ok || got.Pool != "b" {
		t.Fatalf("latest pool lookup incorrect: %+v %v", got, ok)
	}
}
