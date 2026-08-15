package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateConnectionSteps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": "demo-model"}}})
		case "/v1/chat/completions":
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]any{"content": "OK"}}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result := ValidateConnection(context.Background(), ProviderInput{ID: "demo", BaseURL: server.URL, APIKey: "good-key"})
	if !result.OK || !result.Network.OK || !result.Auth.OK || !result.Models.OK || !result.Chat.OK || result.ModelCount != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.NormalizedBaseURL != server.URL+"/v1" {
		t.Fatalf("normalized=%q", result.NormalizedBaseURL)
	}

	bad := ValidateConnection(context.Background(), ProviderInput{ID: "demo", BaseURL: server.URL, APIKey: "bad-key"})
	if bad.OK || !bad.Network.OK || bad.Auth.OK || bad.ErrorType != "auth" {
		t.Fatalf("unexpected auth result: %+v", bad)
	}
}

func TestValidateConnectionRejectsInvalidChatEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": "m"}}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"unexpected": true})
	}))
	defer server.Close()
	result := ValidateConnection(context.Background(), ProviderInput{ID: "demo", BaseURL: server.URL, APIKey: "key"})
	if result.OK || !result.Models.OK || result.Chat.OK || result.ErrorType != "invalid_chat" {
		t.Fatalf("unexpected result: %+v", result)
	}
}
