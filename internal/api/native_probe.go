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
	Configs map[string]provider.Config
	Manager *catalog.Manager
	Gate    map[string]func() bool
}

func (p ProbeRuntime) Allowed(providerID string) bool {
	if gate, ok := p.Gate[providerID]; ok && gate != nil {
		return gate()
	}
	return true
}

func (p ProbeRuntime) Probe(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	id, name := r.URL.Query().Get("provider"), r.URL.Query().Get("model")
	if !p.Allowed(id) {
		http.Error(w, "provider not authorized", http.StatusForbidden)
		return
	}
	c, ok := p.Configs[id]
	if !ok {
		http.Error(w, "provider not found", 404)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	body := map[string]any{"model": name, "messages": []any{map[string]any{"role": "user", "content": "Return exactly JSON: {\"ok\":true}"}}, "max_tokens": 8, "response_format": map[string]string{"type": "json_object"}}
	b, _ := json.Marshal(body)
	req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(c.BaseURL, "/")+"/chat/completions", strings.NewReader(string(b)))
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	res, e := http.DefaultClient.Do(req)
	if e != nil {
		http.Error(w, e.Error(), 502)
		return
	}
	defer res.Body.Close()
	io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
	okJSON := res.StatusCode >= 200 && res.StatusCode < 300
	if p.Manager != nil {
		p.Manager.UpdateProviderModel(name, id, func(m *catalog.Model) {
			now := time.Now()
			if m.CapabilityEvidence == nil {
				m.CapabilityEvidence = map[string]catalog.Evidence{}
			}
			m.CapabilityEvidence["structured_output"] = catalog.Evidence{Supported: okJSON, Confidence: 1, Source: "probe", Level: "runtime", CheckedAt: now}
			m.StructuredOutput = okJSON
			m.StructuredOutputKnown = true
			m.UpdatedAt = now
		})
	}
	writeJSON(w, map[string]any{"provider": id, "model": name, "json_mode": okJSON, "status": res.StatusCode})
}

func (p ProbeRuntime) probeModel(ctx context.Context, providerID, modelName string) (bool, int, error) {
	if !p.Allowed(providerID) {
		return false, http.StatusForbidden, errors.New("provider not authorized")
	}
	c, ok := p.Configs[providerID]
	if !ok {
		return false, http.StatusNotFound, errors.New("provider not found")
	}
	body := map[string]any{"model": modelName, "messages": []any{map[string]any{"role": "user", "content": "Return exactly JSON: {\"ok\":true}"}}, "max_tokens": 8, "response_format": map[string]string{"type": "json_object"}}
	b, err := json.Marshal(body)
	if err != nil {
		return false, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/chat/completions", strings.NewReader(string(b)))
	if err != nil {
		return false, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, 0, err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
	okJSON := res.StatusCode >= 200 && res.StatusCode < 300
	if p.Manager != nil {
		p.Manager.UpdateProviderModel(modelName, providerID, func(m *catalog.Model) {
			now := time.Now()
			if m.CapabilityEvidence == nil {
				m.CapabilityEvidence = map[string]catalog.Evidence{}
			}
			m.CapabilityEvidence["structured_output"] = catalog.Evidence{Supported: okJSON, Confidence: 1, Source: "probe", Level: "runtime", CheckedAt: now}
			m.StructuredOutput = okJSON
			m.StructuredOutputKnown = true
			m.UpdatedAt = now
		})
	}
	return okJSON, res.StatusCode, nil
}
