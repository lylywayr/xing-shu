package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"xing-shu/internal/clientkey"
	"xing-shu/internal/observability"
)

type ClientKeys struct {
	Store *clientkey.Store
	Audit *observability.Audit
}

func (h *ClientKeys) record(action, id string) {
	if h.Audit != nil {
		h.Audit.Record(observability.Event{Time: time.Now().UTC(), Action: action, Model: "client_key/" + id})
	}
}
func (h *ClientKeys) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	writeJSON(w, map[string]any{"items": h.Store.List(), "mode": h.Store.Mode()})
}
func (h *ClientKeys) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var input clientkey.CreateInput
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&input) != nil {
		http.Error(w, "invalid JSON body", 400)
		return
	}
	key, secret, err := h.Store.Create(input)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	h.record("client_key.create", key.ID)
	writeJSON(w, map[string]any{"key": key, "secret": secret, "shown_once": true})
}
func keyID(r *http.Request) string { return strings.TrimSpace(r.URL.Query().Get("id")) }
func (h *ClientKeys) Revoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	id := keyID(r)
	if err := h.Store.Revoke(id); err != nil {
		http.Error(w, "client key not found", 404)
		return
	}
	h.record("client_key.revoke", id)
	writeJSON(w, map[string]any{"ok": true})
}
func (h *ClientKeys) Enable(w http.ResponseWriter, r *http.Request)  { h.enable(w, r, true) }
func (h *ClientKeys) Disable(w http.ResponseWriter, r *http.Request) { h.enable(w, r, false) }
func (h *ClientKeys) enable(w http.ResponseWriter, r *http.Request, enabled bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	id := keyID(r)
	if err := h.Store.SetEnabled(id, enabled); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	h.record(map[bool]string{true: "client_key.enable", false: "client_key.disable"}[enabled], id)
	writeJSON(w, map[string]any{"ok": true})
}
func (h *ClientKeys) Rotate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	id := keyID(r)
	key, secret, err := h.Store.Rotate(id)
	if err != nil {
		http.Error(w, "client key not found", 404)
		return
	}
	h.record("client_key.rotate", id)
	writeJSON(w, map[string]any{"key": key, "secret": secret, "shown_once": true})
}
func (h *ClientKeys) Mode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var input struct {
		Mode clientkey.Mode `json:"mode"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil {
		http.Error(w, "invalid JSON body", 400)
		return
	}
	if err := h.Store.SetMode(input.Mode); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	h.record("client_key.mode", string(input.Mode))
	writeJSON(w, map[string]any{"ok": true, "mode": input.Mode})
}
func ClientAPIInfo(baseURL string, store *clientkey.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", 405)
			return
		}
		base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
		if base == "" {
			scheme := "http"
			if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
				scheme = "https"
			}
			host := r.Host
			if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0]); forwarded != "" {
				host = forwarded
			}
			base = scheme + "://" + host
		}
		writeJSON(w, map[string]any{"base_url": base + "/v1", "models_endpoint": base + "/v1/models", "chat_endpoint": base + "/v1/chat/completions", "authentication": "Bearer client API key", "mode": store.Mode(), "scopes": []string{"models:read", "chat:write"}})
	}
}
