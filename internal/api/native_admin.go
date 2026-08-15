package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
	"xing-shu/internal/catalog"
	"xing-shu/internal/provider"
)

type ProviderRuntime struct {
	Configs      map[string]provider.Config
	ConfigSource func() map[string]provider.Config
	Manager      *catalog.Manager
	Ops          *OpsState
	SyncGate     map[string]func() bool
}

func (p ProviderRuntime) configs() map[string]provider.Config {
	if p.ConfigSource != nil {
		return p.ConfigSource()
	}
	return p.Configs
}

func (p ProviderRuntime) List() []map[string]any {
	out := []map[string]any{}
	for id, c := range p.configs() {
		if c.Source == provider.SourceExternal {
			continue
		}
		disabled := p.Ops != nil && p.Ops.IsDisabled(id)
		status := "active"
		if disabled {
			status = "disabled"
		}
		cooldownUntil := time.Time{}
		cooldownActive := false
		if p.Ops != nil {
			cooldownUntil, cooldownActive = p.Ops.Cooldown(id)
			if cooldownActive && !disabled {
				status = "cooldown"
			}
		}
		lastSync := ""
		if p.Manager != nil {
			if value := p.Manager.Last(id); !value.IsZero() {
				lastSync = value.UTC().Format(time.RFC3339)
			}
		}
		item := map[string]any{"id": id, "base_url": c.BaseURL, "kind": c.Kind, "models": len(p.models(id)), "disabled": disabled, "status": status, "last_sync": lastSync, "cooldown_active": cooldownActive, "sync_error": "", "sync_error_type": ""}
		if p.Ops != nil {
			if syncError, ok := p.Ops.SyncError(id); ok {
				item["sync_error"] = syncError.Message
				item["sync_error_type"] = syncError.Type
				if !disabled && !cooldownActive {
					status = "error"
					item["status"] = status
				}
			}
		}
		if cooldownActive {
			item["cooldown_until"] = cooldownUntil.UTC().Format(time.RFC3339)
		}
		out = append(out, item)
	}
	return out
}
func (p ProviderRuntime) models(id string) []catalog.Model {
	if p.Manager == nil {
		return nil
	}
	all := p.Manager.Snapshot().Models
	out := []catalog.Model{}
	for _, m := range all {
		if m.Provider == id {
			out = append(out, m)
		}
	}
	return out
}
func (p ProviderRuntime) Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", 405)
		return
	}
	writeJSON(w, map[string]any{"providers": p.List()})
}
func (p ProviderRuntime) Sync(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	id := r.URL.Query().Get("id")
	results := map[string]any{}
	for name, c := range p.configs() {
		if c.Source == provider.SourceExternal {
			continue
		}
		if id != "" && id != name {
			continue
		}
		if gate, ok := p.SyncGate[name]; ok && gate != nil && !gate() {
			results[name] = map[string]any{"error_type": "integration_not_authorized"}
			continue
		}
		res := p.Manager.Sync(ctx, c)
		if res.ErrorType == "" && p.Manager != nil {
			p.Manager.MarkSynced(name, time.Now())
			if p.Ops != nil {
				p.Ops.ClearSyncError(name)
			}
		} else if p.Ops != nil {
			p.Ops.MarkSyncError(name, string(res.ErrorType), res.Message)
		}
		results[name] = res
	}
	writeJSON(w, map[string]any{"ok": true, "results": results})
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
