package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"xing-shu/internal/catalog"
	"xing-shu/internal/provider"
)

type ProviderRegistryRuntime struct {
	Registry *provider.Registry
	Manager  *catalog.Manager
}

type providerMutationRequest struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Kind    string `json:"kind"`
	Enabled *bool  `json:"enabled,omitempty"`
}

func (p *ProviderRegistryRuntime) configs() map[string]provider.Config {
	if p == nil || p.Registry == nil {
		return map[string]provider.Config{}
	}
	return p.Registry.Snapshot()
}
func (p *ProviderRegistryRuntime) reconcile() {
	if p != nil && p.Manager != nil {
		p.Manager.ReconcileProviders(p.configs())
	}
}

func (p *ProviderRegistryRuntime) sync(id string) provider.Result {
	if p == nil || p.Manager == nil || p.Registry == nil {
		return provider.Result{ErrorType: provider.NetworkError, Message: "provider runtime unavailable"}
	}
	config, ok := p.Registry.Config(id)
	if !ok {
		return provider.Result{ErrorType: provider.NetworkError, Message: "provider is disabled"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return p.Manager.Sync(ctx, config)
}

func (p *ProviderRegistryRuntime) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	items := []provider.ProviderView{}
	ready := false
	if p != nil && p.Registry != nil {
		items = p.Registry.Views()
		ready = p.Registry.Ready()
	}
	writeJSON(w, map[string]any{"items": items, "credential_key_configured": ready})
}
func (p *ProviderRegistryRuntime) Validate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	input, ok := decodeProviderMutation(w, r)
	if !ok {
		return
	}
	if input.ID != "" && input.APIKey == "" && p != nil && p.Registry != nil {
		resolved, err := p.Registry.ResolveInput(input.ID, toProviderInput(input))
		if err == nil {
			input = fromProviderInput(resolved)
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
	defer cancel()
	result := provider.ValidateConnection(ctx, toProviderInput(input))
	writeJSON(w, result)
}
func (p *ProviderRegistryRuntime) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	input, ok := decodeProviderMutation(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
	defer cancel()
	validation := provider.ValidateConnection(ctx, toProviderInput(input))
	if !validation.OK {
		w.WriteHeader(http.StatusUnprocessableEntity)
		writeJSON(w, map[string]any{"ok": false, "validation": validation})
		return
	}
	input.BaseURL = validation.NormalizedBaseURL
	view, err := p.Registry.Create(toProviderInput(input))
	if writeProviderError(w, err) {
		return
	}
	p.reconcile()
	syncResult := p.sync(view.ID)
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{"ok": true, "provider": view, "validation": validation, "sync": syncResult})
}
func (p *ProviderRegistryRuntime) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	input, ok := decodeProviderMutation(w, r)
	if !ok {
		return
	}
	input.ID = id
	resolved, err := p.Registry.ResolveInput(id, toProviderInput(input))
	if writeProviderError(w, err) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
	defer cancel()
	validation := provider.ValidateConnection(ctx, resolved)
	if !validation.OK {
		w.WriteHeader(http.StatusUnprocessableEntity)
		writeJSON(w, map[string]any{"ok": false, "validation": validation})
		return
	}
	input.BaseURL = validation.NormalizedBaseURL
	view, err := p.Registry.Update(id, toProviderInput(input))
	if writeProviderError(w, err) {
		return
	}
	p.reconcile()
	syncResult := p.sync(view.ID)
	writeJSON(w, map[string]any{"ok": true, "provider": view, "validation": validation, "sync": syncResult})
}
func (p *ProviderRegistryRuntime) Enable(w http.ResponseWriter, r *http.Request) {
	p.setEnabled(w, r, true)
}
func (p *ProviderRegistryRuntime) Disable(w http.ResponseWriter, r *http.Request) {
	p.setEnabled(w, r, false)
}
func (p *ProviderRegistryRuntime) setEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	view, err := p.Registry.SetEnabled(strings.TrimSpace(r.URL.Query().Get("id")), enabled)
	if writeProviderError(w, err) {
		return
	}
	p.reconcile()
	var syncResult provider.Result
	if enabled {
		syncResult = p.sync(view.ID)
	}
	writeJSON(w, map[string]any{"ok": true, "provider": view, "sync": syncResult})
}
func (p *ProviderRegistryRuntime) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if writeProviderError(w, p.Registry.Delete(id)) {
		return
	}
	p.reconcile()
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func decodeProviderMutation(w http.ResponseWriter, r *http.Request) (providerMutationRequest, bool) {
	var input providerMutationRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32*1024)).Decode(&input) != nil {
		http.Error(w, "invalid JSON body", 400)
		return input, false
	}
	return input, true
}
func toProviderInput(x providerMutationRequest) provider.ProviderInput {
	return provider.ProviderInput{ID: x.ID, Name: x.Name, BaseURL: x.BaseURL, APIKey: x.APIKey, Kind: x.Kind, Enabled: x.Enabled}
}
func fromProviderInput(x provider.ProviderInput) providerMutationRequest {
	return providerMutationRequest{ID: x.ID, Name: x.Name, BaseURL: x.BaseURL, APIKey: x.APIKey, Kind: x.Kind, Enabled: x.Enabled}
}
func writeProviderError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	status := http.StatusBadRequest
	if errors.Is(err, provider.ErrNotFound) {
		status = 404
	}
	if errors.Is(err, provider.ErrDuplicate) {
		status = 409
	}
	if errors.Is(err, provider.ErrReadOnly) {
		status = 403
	}
	if errors.Is(err, provider.ErrKeyUnavailable) {
		status = 503
	}
	http.Error(w, err.Error(), status)
	return true
}
