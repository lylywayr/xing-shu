package routing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"xing-shu/internal/catalog"
	"xing-shu/internal/provider"
	"xing-shu/internal/runtime"
)

func TestCompleteReliableFailsOverAndRecordsAttempts(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "limited", 429) }))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer good.Close()
	s := &Service{Providers: map[string]ProviderConfig{"p1": {ID: "p1", BaseURL: bad.URL}, "p2": {ID: "p2", BaseURL: good.URL}}, Models: []catalog.Model{{ID: "a", Provider: "p1", Status: catalog.Active, Admitted: true, AutoRoutable: true, Score: 10}, {ID: "b", Provider: "p2", Status: catalog.Active, Admitted: true, AutoRoutable: true, Score: 9}}, Client: provider.NewChatClient(), Health: NewHealth(HealthOptions{})}
	result, err := s.CompleteReliable(context.Background(), []byte(`{"model":"auto","messages":[]}`), ReliableOptions{MaxAttempts: 3, PerAttemptTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Response.Body.Close()
	if result.Model != "b" || len(result.Attempts) != 2 || result.Attempts[0].Status != 429 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if s.Health.Available("p1", "a", time.Now()) {
		t.Fatal("rate-limited route did not cool down")
	}
}

func TestCombinedCapabilitiesRequireOneModelToSupportAll(t *testing.T) {
	body := []byte(`{"tools":[{}],"response_format":{"type":"json_object"},"messages":[]}`)
	s := &Service{Providers: map[string]ProviderConfig{"p": {ID: "p", BaseURL: "http://x"}}, Models: []catalog.Model{{ID: "tools", Provider: "p", Status: catalog.Active, Admitted: true, AutoRoutable: true, Tools: true}, {ID: "json", Provider: "p", Status: catalog.Active, Admitted: true, AutoRoutable: true, StructuredOutput: true}}}
	if got := s.RankedCandidates(body); len(got) != 0 {
		t.Fatalf("partial capability model accepted: %#v", got)
	}
}

func TestReliableRecordsEachProviderModelAttemptForLearning(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", 500) }))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"ok":true}`)) }))
	defer good.Close()
	learning := runtime.NewLearning()
	s := &Service{Providers: map[string]ProviderConfig{"p1": {ID: "p1", BaseURL: bad.URL}, "p2": {ID: "p2", BaseURL: good.URL}}, Models: []catalog.Model{{ID: "same", Provider: "p1", Status: catalog.Active, Admitted: true, AutoRoutable: true, Score: 10}, {ID: "same", Provider: "p2", Status: catalog.Active, Admitted: true, AutoRoutable: true, Score: 9}}, Client: provider.NewChatClient(), Health: NewHealth(HealthOptions{}), Learning: learning}
	if _, err := s.CompleteReliable(context.Background(), []byte(`{"model":"auto","messages":[]}`), ReliableOptions{MaxAttempts: 2, PerAttemptTimeout: time.Second}); err != nil {
		t.Fatal(err)
	}
	stats := learning.Snapshot()
	if stats["p1/same"].Failures != 1 || stats["p2/same"].Success != 1 {
		t.Fatalf("attempt learning not isolated: %#v", stats)
	}
}

func TestReliableSkipsOtherModelsFromFailedProvider(t *testing.T) {
	badCalls := 0
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { badCalls++; http.Error(w, "down", 500) }))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"ok":true}`)) }))
	defer good.Close()
	s := &Service{Providers: map[string]ProviderConfig{"p1": {ID: "p1", BaseURL: bad.URL}, "p2": {ID: "p2", BaseURL: good.URL}}, Models: []catalog.Model{{ID: "a", Provider: "p1", Status: catalog.Active, Admitted: true, AutoRoutable: true, Score: 30}, {ID: "a2", Provider: "p1", Status: catalog.Active, Admitted: true, AutoRoutable: true, Score: 20}, {ID: "b", Provider: "p2", Status: catalog.Active, Admitted: true, AutoRoutable: true, Score: 10}}, Client: provider.NewChatClient(), Health: NewHealth(HealthOptions{})}
	result, err := s.CompleteReliable(context.Background(), []byte(`{"model":"auto","messages":[]}`), ReliableOptions{MaxAttempts: 3, PerAttemptTimeout: time.Second})
	if err != nil || result.Model != "b" || badCalls != 1 {
		t.Fatalf("provider was retried: result=%#v err=%v calls=%d", result, err, badCalls)
	}
}

func TestCandidatesRequireApprovalCapabilityAndHealth(t *testing.T) {
	h := NewHealth(HealthOptions{})
	h.Fail("p1", "tools", FailureRateLimited, time.Now())
	s := &Service{Providers: map[string]ProviderConfig{"p1": {ID: "p1", BaseURL: "http://x"}, "p2": {ID: "p2", BaseURL: "http://x"}}, Models: []catalog.Model{{ID: "tools", Provider: "p1", Status: catalog.Active, Admitted: true, AutoRoutable: true, Capabilities: []string{"tools"}}, {ID: "unapproved", Provider: "p2", Status: catalog.Active, AutoRoutable: false, Capabilities: []string{"tools"}}}, Health: h}
	got := s.RankedCandidates([]byte(`{"model":"auto","tools":[{}]}`))
	if len(got) != 0 {
		t.Fatalf("ineligible candidate returned: %#v", got)
	}
}
