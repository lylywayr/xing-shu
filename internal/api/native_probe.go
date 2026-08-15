package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"xing-shu/internal/catalog"
	"xing-shu/internal/provider"
)

type ProbeRuntime struct {
	Configs      map[string]provider.Config
	ConfigSource func() map[string]provider.Config
	Manager      *catalog.Manager
	Gate         map[string]func() bool
}
type capabilityProbe struct {
	Supported  bool
	Conclusive bool
	Status     int
	Err        error
}
type capabilityProbeResult struct {
	OK           bool                        `json:"ok"`
	Status       int                         `json:"status"`
	Error        string                      `json:"error,omitempty"`
	Capabilities map[string]catalog.Evidence `json:"capabilities"`
}

func (p ProbeRuntime) configs() map[string]provider.Config {
	if p.ConfigSource != nil {
		return p.ConfigSource()
	}
	return p.Configs
}
func (p ProbeRuntime) Allowed(providerID string) bool {
	if gate, ok := p.Gate[providerID]; ok && gate != nil {
		return gate()
	}
	return true
}
func (p ProbeRuntime) Probe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	providerID, modelID := r.URL.Query().Get("provider"), r.URL.Query().Get("model")
	result := p.probeCapabilities(ctx, providerID, modelID)
	if result.Error != "" {
		http.Error(w, result.Error, result.Status)
		return
	}
	writeJSON(w, map[string]any{"provider": providerID, "model": modelID, "ok": result.OK, "capabilities": result.Capabilities, "status": result.Status})
}
func (p ProbeRuntime) probeCapabilities(ctx context.Context, providerID, modelID string) capabilityProbeResult {
	if !p.Allowed(providerID) {
		return capabilityProbeResult{Status: http.StatusForbidden, Error: "provider not authorized"}
	}
	config, ok := p.configs()[providerID]
	if !ok {
		return capabilityProbeResult{Status: http.StatusNotFound, Error: "provider not found"}
	}
	endpoint := strings.TrimRight(config.BaseURL, "/") + "/chat/completions"
	checks := []struct {
		name string
		body map[string]any
	}{
		{"structured_output", map[string]any{"model": modelID, "messages": []any{map[string]any{"role": "user", "content": "Return exactly JSON: {\"ok\":true}"}}, "max_tokens": 16, "response_format": map[string]string{"type": "json_object"}}},
		{"tools", map[string]any{"model": modelID, "messages": []any{map[string]any{"role": "user", "content": "Call the ping function now."}}, "max_tokens": 16, "tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "ping", "description": "Return pong", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}}}}, "tool_choice": map[string]any{"type": "function", "function": map[string]string{"name": "ping"}}}},
		{"vision", map[string]any{"model": modelID, "messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "Reply with one word."}, map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw=="}}}}}, "max_tokens": 8}},
	}
	evidence := make(map[string]catalog.Evidence, len(checks))
	status := http.StatusOK
	for _, check := range checks {
		probe := executeProbe(ctx, endpoint, config.APIKey, check.name, check.body)
		if probe.Err != nil || !probe.Conclusive {
			return capabilityProbeResult{Status: http.StatusBadGateway, Error: "capability probe unavailable"}
		}
		if probe.Status > status {
			status = probe.Status
		}
		evidence[check.name] = catalog.Evidence{Supported: probe.Supported, Confidence: 1, Source: "runtime_probe", Level: "protocol", CheckedAt: time.Now().UTC()}
	}
	if p.Manager != nil {
		p.Manager.UpdateProviderModel(modelID, providerID, func(model *catalog.Model) {
			if model.CapabilityEvidence == nil {
				model.CapabilityEvidence = map[string]catalog.Evidence{}
			}
			for capability, item := range evidence {
				model.CapabilityEvidence[capability] = item
			}
			model.StructuredOutput, model.StructuredOutputKnown = evidence["structured_output"].Supported, true
			model.Tools, model.Vision = evidence["tools"].Supported, evidence["vision"].Supported
			model.UpdatedAt = time.Now().UTC()
		})
	}
	return capabilityProbeResult{OK: true, Status: status, Capabilities: evidence}
}
func executeProbe(ctx context.Context, endpoint, key, capability string, body map[string]any) capabilityProbe {
	encoded, err := json.Marshal(body)
	if err != nil {
		return capabilityProbe{Err: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(encoded)))
	if err != nil {
		return capabilityProbe{Err: err}
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return capabilityProbe{Err: err}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return capabilityProbe{Err: err}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		conclusive := res.StatusCode >= 400 && res.StatusCode < 500 && res.StatusCode != http.StatusUnauthorized && res.StatusCode != http.StatusForbidden && res.StatusCode != http.StatusRequestTimeout && res.StatusCode != http.StatusTooManyRequests
		return capabilityProbe{Status: res.StatusCode, Conclusive: conclusive}
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content   any `json:"content"`
				ToolCalls []struct {
					Function struct {
						Name string `json:"name"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &envelope) != nil || len(envelope.Choices) == 0 {
		return capabilityProbe{Status: res.StatusCode, Conclusive: false}
	}
	message := envelope.Choices[0].Message
	supported := true
	switch capability {
	case "structured_output":
		text, ok := message.Content.(string)
		if !ok || json.Unmarshal([]byte(text), &map[string]any{}) != nil {
			supported = false
		}
	case "tools":
		supported = false
		for _, call := range message.ToolCalls {
			if call.Function.Name == "ping" {
				supported = true
				break
			}
		}
	}
	return capabilityProbe{Supported: supported, Conclusive: true, Status: res.StatusCode}
}
func (p ProbeRuntime) probeModel(ctx context.Context, providerID, modelID string) (bool, int, error) {
	result := p.probeCapabilities(ctx, providerID, modelID)
	if result.Error != "" {
		return false, result.Status, errors.New(result.Error)
	}
	return result.Capabilities["structured_output"].Supported, result.Status, nil
}
