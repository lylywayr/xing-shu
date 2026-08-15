package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"xing-shu/internal/catalog"
	"xing-shu/internal/provider"
)

type ReviewerConnectionRuntime struct {
	Configs      map[string]provider.Config
	ConfigSource func() map[string]provider.Config
	Manager      *catalog.Manager
}

func (r *ReviewerConnectionRuntime) configs() map[string]provider.Config {
	if r.ConfigSource != nil {
		return r.ConfigSource()
	}
	return r.Configs
}

type reviewerConnectionRequest struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

func (r *ReviewerConnectionRuntime) Test(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var input reviewerConnectionRequest
	if json.NewDecoder(http.MaxBytesReader(w, req.Body, 32*1024)).Decode(&input) != nil {
		http.Error(w, "invalid JSON body", 400)
		return
	}
	model, ok := r.find(input.Provider, input.Model)
	if !ok {
		http.Error(w, "active model not found", 400)
		return
	}
	config, ok := r.configs()[input.Provider]
	if !ok {
		http.Error(w, "provider not found", 400)
		return
	}
	result := map[string]any{"provider": input.Provider, "model": input.Model, "network": false, "auth": false, "http": false, "json": false, "structured_output": false, "latency_ms": 0, "error_type": ""}
	started := time.Now()
	ctx, cancel := context.WithTimeout(req.Context(), 45*time.Second)
	defer cancel()
	payload := map[string]any{"model": input.Model, "messages": []any{map[string]string{"role": "user", "content": "Return exactly JSON: {\"ok\":true}"}}, "max_tokens": 16, "response_format": map[string]string{"type": "json_object"}}
	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(config.BaseURL, "/")+"/chat/completions", strings.NewReader(string(body)))
	if err != nil {
		result["error_type"] = "network"
		result["error"] = err.Error()
		writeJSON(w, result)
		return
	}
	httpReq.Header.Set("Authorization", "Bearer "+config.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(httpReq)
	result["latency_ms"] = time.Since(started).Milliseconds()
	result["network"] = err == nil
	if err != nil {
		result["error_type"] = "network"
		result["error"] = err.Error()
		writeJSON(w, result)
		return
	}
	defer res.Body.Close()
	result["status"] = res.StatusCode
	result["http"] = res.StatusCode >= 200 && res.StatusCode < 300
	result["auth"] = res.StatusCode != http.StatusUnauthorized && res.StatusCode != http.StatusForbidden
	if !result["http"].(bool) {
		result["error_type"] = classifyReviewerHTTP(res.StatusCode)
		result["error"] = fmt.Sprintf("provider http %d", res.StatusCode)
		writeJSON(w, result)
		return
	}
	raw, readErr := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if readErr != nil {
		result["error_type"] = "network"
		result["error"] = readErr.Error()
		writeJSON(w, result)
		return
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &envelope) != nil || len(envelope.Choices) == 0 {
		result["error_type"] = "json"
		result["error"] = "provider response is not valid chat JSON"
		writeJSON(w, result)
		return
	}
	result["json"] = true
	var content map[string]any
	if json.Unmarshal([]byte(envelope.Choices[0].Message.Content), &content) == nil {
		result["structured_output"] = true
	} else {
		result["error_type"] = "structured_output"
		result["error"] = "message content is not strict JSON"
	}
	if !result["structured_output"].(bool) {
		writeJSON(w, result)
		return
	}
	writeJSON(w, result)
	_ = model
}
func (r *ReviewerConnectionRuntime) find(providerID, modelID string) (catalog.Model, bool) {
	if r.Manager == nil {
		return catalog.Model{}, false
	}
	for _, model := range r.Manager.Snapshot().Models {
		if model.Provider == providerID && model.ID == modelID && model.Status == catalog.Active {
			return model, true
		}
	}
	return catalog.Model{}, false
}
func classifyReviewerHTTP(status int) string {
	if status == 401 || status == 403 {
		return "auth"
	}
	if status >= 500 {
		return "upstream_5xx"
	}
	if status == 429 {
		return "rate_limited"
	}
	return "http"
}
