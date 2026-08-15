package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ValidationStep struct {
	OK        bool   `json:"ok"`
	Status    int    `json:"status,omitempty"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Message   string `json:"message,omitempty"`
}

type ValidationResult struct {
	OK                bool           `json:"ok"`
	NormalizedBaseURL string         `json:"normalized_base_url,omitempty"`
	ModelCount        int            `json:"model_count"`
	SampleModel       string         `json:"sample_model,omitempty"`
	ErrorType         string         `json:"error_type,omitempty"`
	Network           ValidationStep `json:"network"`
	Auth              ValidationStep `json:"auth"`
	Models            ValidationStep `json:"models"`
	Chat              ValidationStep `json:"chat"`
}

func ValidateConnection(ctx context.Context, input ProviderInput) ValidationResult {
	result := ValidationResult{}
	normalized, err := NormalizeBaseURL(input.BaseURL)
	if err != nil {
		result.ErrorType = "invalid_url"
		result.Network.Message = err.Error()
		return result
	}
	result.NormalizedBaseURL = normalized
	client := &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	modelsURL := normalized + "/models"
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		result.ErrorType = "network"
		result.Network.Message = err.Error()
		return result
	}
	setBearer(req, input.APIKey)
	resp, err := client.Do(req)
	result.Network.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		result.ErrorType = "network"
		result.Network.Message = safeError(err)
		return result
	}
	result.Network.OK = true
	result.Network.Status = resp.StatusCode
	result.Auth.Status = resp.StatusCode
	result.Models.Status = resp.StatusCode
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	resp.Body.Close()
	if readErr != nil {
		result.ErrorType = "network"
		result.Models.Message = readErr.Error()
		return result
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		result.ErrorType = "auth"
		result.Auth.Message = "API Key rejected"
		return result
	}
	result.Auth.OK = true
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.ErrorType = string(Classify(resp.StatusCode, nil))
		result.Models.Message = fmt.Sprintf("models endpoint returned HTTP %d", resp.StatusCode)
		return result
	}
	var catalog struct {
		Data []RawModel `json:"data"`
	}
	if json.Unmarshal(body, &catalog) != nil || len(catalog.Data) == 0 {
		result.ErrorType = "invalid_catalog"
		result.Models.Message = "models response must contain a non-empty data array"
		return result
	}
	result.Models.OK = true
	result.ModelCount = len(catalog.Data)
	result.SampleModel = catalog.Data[0].ID
	payload, _ := json.Marshal(map[string]any{"model": result.SampleModel, "messages": []map[string]string{{"role": "user", "content": "Reply with OK"}}, "max_tokens": 8, "stream": false})
	chatReq, err := http.NewRequestWithContext(ctx, http.MethodPost, normalized+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		result.ErrorType = "network"
		result.Chat.Message = err.Error()
		return result
	}
	setBearer(chatReq, input.APIKey)
	chatReq.Header.Set("Content-Type", "application/json")
	start = time.Now()
	chatResp, err := client.Do(chatReq)
	result.Chat.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		result.ErrorType = "network"
		result.Chat.Message = safeError(err)
		return result
	}
	result.Chat.Status = chatResp.StatusCode
	chatBody, readErr := io.ReadAll(io.LimitReader(chatResp.Body, 2<<20))
	chatResp.Body.Close()
	if readErr != nil {
		result.ErrorType = "network"
		result.Chat.Message = readErr.Error()
		return result
	}
	if chatResp.StatusCode == http.StatusUnauthorized || chatResp.StatusCode == http.StatusForbidden {
		result.ErrorType = "auth"
		result.Auth.OK = false
		result.Chat.Message = "API Key rejected by chat endpoint"
		return result
	}
	if chatResp.StatusCode < 200 || chatResp.StatusCode >= 300 {
		result.ErrorType = string(Classify(chatResp.StatusCode, nil))
		result.Chat.Message = fmt.Sprintf("chat endpoint returned HTTP %d", chatResp.StatusCode)
		return result
	}
	var envelope struct {
		Choices []json.RawMessage `json:"choices"`
	}
	if json.Unmarshal(chatBody, &envelope) != nil || len(envelope.Choices) == 0 {
		result.ErrorType = "invalid_chat"
		result.Chat.Message = "chat response must contain choices"
		return result
	}
	result.Chat.OK = true
	result.OK = true
	return result
}
func setBearer(req *http.Request, key string) {
	if strings.TrimSpace(key) != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
}
func safeError(err error) string {
	message := err.Error()
	if len(message) > 300 {
		message = message[:300]
	}
	return message
}
