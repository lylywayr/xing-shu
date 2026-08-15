package routing

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"xing-shu/internal/catalog"
	"xing-shu/internal/provider"
)

func TestStreamingFailsOverBeforeFirstSSEEvent(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "not-sse\n")
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[]}\n\ndata: [DONE]\n\n")
	}))
	defer good.Close()
	s := &Service{Providers: map[string]ProviderConfig{"bad": {ID: "bad", BaseURL: bad.URL}, "good": {ID: "good", BaseURL: good.URL}}, Models: []catalog.Model{{ID: "a", Provider: "bad", Status: catalog.Active, Admitted: true, AutoRoutable: true, Score: 10}, {ID: "b", Provider: "good", Status: catalog.Active, Admitted: true, AutoRoutable: true, Score: 9}}, Client: provider.NewChatClient(), Health: NewHealth(HealthOptions{})}
	result, err := s.CompleteReliable(context.Background(), []byte(`{"model":"auto","stream":true,"messages":[]}`), ReliableOptions{MaxAttempts: 2, PerAttemptTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Response.Body.Close()
	body, _ := io.ReadAll(result.Response.Body)
	if result.Model != "b" || len(result.Attempts) != 2 || !strings.Contains(string(body), "[DONE]") {
		t.Fatalf("unexpected stream result: %#v %s", result, string(body))
	}
}

func TestHealthPersistsAndRequiresProbe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "health.json")
	now := time.Now()
	h := OpenHealth(path, HealthOptions{NetworkCooldown: time.Millisecond})
	h.Fail("p", "m", FailureNetwork, now)
	loaded := OpenHealth(path, HealthOptions{NetworkCooldown: time.Millisecond})
	if loaded.Available("p", "m", now.Add(time.Second)) {
		t.Fatal("expired cooldown bypassed required recovery probe")
	}
	if len(loaded.DueForProbe(now.Add(time.Second))) != 1 {
		t.Fatal("route not due for recovery probe")
	}
	loaded.Success("p", "m", now.Add(time.Second))
	if !loaded.Available("p", "m", now.Add(time.Second)) {
		t.Fatal("successful probe did not restore route")
	}
}
