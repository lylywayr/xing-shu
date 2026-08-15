package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"xing-shu/internal/catalog"
	"xing-shu/internal/provider"
)

func TestProbeCapabilitiesPersistsProtocolEvidence(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		message := map[string]any{"content": `{"ok":true}`}
		if _, hasTools := body["tools"]; hasTools {
			message = map[string]any{"tool_calls": []any{map[string]any{"function": map[string]any{"name": "ping"}}}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": message}}})
	}))
	defer upstream.Close()
	manager := catalog.NewManager(catalog.Catalog{Models: []catalog.Model{{ID: "m", Provider: "p", Status: catalog.Active, Admitted: true}}}, nil)
	runtime := ProbeRuntime{Configs: map[string]provider.Config{"p": {ID: "p", BaseURL: upstream.URL}}, Manager: manager}
	result := runtime.probeCapabilities(context.Background(), "p", "m")
	if !result.OK || len(result.Capabilities) != 3 || !result.Capabilities["structured_output"].Supported || !result.Capabilities["tools"].Supported || !result.Capabilities["vision"].Supported {
		t.Fatalf("unexpected probe result: %+v", result)
	}
	model := manager.Snapshot().Models[0]
	for _, name := range []string{"structured_output", "tools", "vision"} {
		if model.CapabilityEvidence[name].Source != "runtime_probe" {
			t.Fatalf("missing %s evidence: %+v", name, model)
		}
	}
}
func TestProbeCapabilitiesRejectsInconclusiveResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer upstream.Close()
	runtime := ProbeRuntime{Configs: map[string]provider.Config{"p": {ID: "p", BaseURL: upstream.URL}}}
	result := runtime.probeCapabilities(context.Background(), "p", "m")
	if result.Error == "" || result.Status != http.StatusBadGateway {
		t.Fatalf("expected inconclusive failure, got %+v", result)
	}
}
