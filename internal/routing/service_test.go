package routing

import (
	"testing"
	"time"
	"xing-shu/internal/catalog"
	"xing-shu/internal/quota"
	"xing-shu/internal/runtime"
)

func TestExternalIntegrationGateBlocksCallsAndAutoSelection(t *testing.T) {
	gate := false
	s := Service{ProviderGate: map[string]func() bool{"freellmapi": func() bool { return gate }}, Providers: map[string]ProviderConfig{"freellmapi": {ID: "freellmapi", BaseURL: "http://x"}}, Models: []catalog.Model{{ID: "m", Provider: "freellmapi", Status: catalog.Active, AutoRoutable: true, Score: 100}}}
	if s.SelectAuto([]byte(`{"model":"auto","messages":[]}`)) != "" {
		t.Fatal("disabled external integration selected")
	}
	gate = true
	if s.SelectAuto([]byte(`{"model":"auto","messages":[]}`)) != "m" {
		t.Fatal("authorized external integration not selected")
	}
}
func TestDynamicQuotaOnlyExcludesExplicitlyExhausted(t *testing.T) {
	for _, state := range []quota.State{quota.Unknown, quota.Available} {
		s := Service{QuotaProvider: func(string) (quota.Snapshot, bool) { return quota.Snapshot{State: state}, true }, Providers: map[string]ProviderConfig{"p": {ID: "p", BaseURL: "http://x"}}}
		if !s.HasProvider("p") {
			t.Fatalf("state %s must not be treated as exhausted", state)
		}
	}
	s := Service{QuotaProvider: func(string) (quota.Snapshot, bool) { return quota.Snapshot{State: quota.Exhausted}, true }, Providers: map[string]ProviderConfig{"p": {ID: "p", BaseURL: "http://x"}}}
	if s.HasProvider("p") {
		t.Fatal("exhausted provider remained routable")
	}
}
func TestSelectAutoOnlyAllow(t *testing.T) {
	s := Service{Models: []catalog.Model{{ID: "unknown", Provider: "p", Status: catalog.Unknown, AutoRoutable: false}, {ID: "ok", Provider: "p", Status: catalog.Active, AutoRoutable: true, Score: 5}}, Providers: map[string]ProviderConfig{"p": {ID: "p", BaseURL: "http://x"}}}
	if s.SelectAuto([]byte(`{"model":"auto","messages":[]}`)) != "ok" {
		t.Fatal("selected non-active model")
	}
}
func TestUnknownRejected(t *testing.T) {
	s := Service{Models: []catalog.Model{{ID: "unknown", Status: catalog.Unknown, AutoRoutable: false}}}
	if s.HasModel("unknown") {
		t.Fatal("unknown model allowed")
	}
}

func TestKnowledgeBonusMatchesTrustedKnowledge(t *testing.T) {
	expires := time.Now().Add(30 * 24 * time.Hour)
	store := runtime.NewKnowledgeStore()
	store.Upsert(runtime.Knowledge{ID: "k1", Signature: runtime.Signature(runtime.TaskFeatures{NeedsTools: true}), RecommendedModel: "m1", Status: runtime.KnowledgeTrusted, Confidence: 1, ValidatedSamples: 10, SuccessfulSamples: 10, ExpiresAt: &expires})
	service := &Service{Knowledge: store}
	model := catalog.Model{ID: "m1"}
	if got := service.KnowledgeBonus([]byte(`{"tools":[{}],"messages":[]}`), model); got <= 0 {
		t.Fatalf("expected trusted knowledge bonus, got %d", got)
	}
	if got := service.KnowledgeBonus([]byte(`{"tools":[{}],"messages":[{"content":"deploy to production"}]}`), model); got != 0 {
		t.Fatalf("high risk request must not receive knowledge bonus, got %d", got)
	}
}

func TestExpiredExhaustionDoesNotHardBlockProvider(t *testing.T) {
	expired := time.Now().Add(-time.Minute)
	s := Service{QuotaProvider: func(string) (quota.Snapshot, bool) {
		return quota.Snapshot{State: quota.Exhausted, FreshUntil: &expired}, true
	}, Providers: map[string]ProviderConfig{"p": {ID: "p", BaseURL: "http://x"}}}
	if !s.HasProvider("p") {
		t.Fatal("expired exhaustion must become unknown, not hard block")
	}
}
