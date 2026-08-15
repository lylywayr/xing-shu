package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"xing-shu/internal/catalog"
	"xing-shu/internal/integration"
	"xing-shu/internal/observability"
	"xing-shu/internal/quota"
)

type IntegrationRuntime struct {
	FreeLLMAPI *integration.Manager
	Catalog    *catalog.Manager
	Quota      *quota.Manager
	Audit      *observability.Audit
}

func (i *IntegrationRuntime) audit(action, status string) {
	if i != nil && i.Audit != nil {
		i.Audit.Record(observability.Event{Time: time.Now().UTC(), Action: action, Provider: integration.FreeLLMAPIID, Status: map[string]int{"ok": 200, "enabled": 200, "disabled": 200}[status]})
	}
}

func (i *IntegrationRuntime) status(w http.ResponseWriter, _ *http.Request) {
	if i == nil || i.FreeLLMAPI == nil {
		writeJSON(w, map[string]any{"items": []any{}})
		return
	}
	x := i.FreeLLMAPI.State()
	local, localChecked, localError := i.FreeLLMAPI.LocalQuotaState()
	// Local accounting is a post-consent resource. Never include an old snapshot
	// in the status representation when consent is absent.
	if !x.Authorized {
		local, localChecked, localError = nil, time.Time{}, "not_authorized"
	}
	localQuotaState := "not_authorized"
	if x.Authorized {
		localQuotaState = "unavailable"
		if local != nil {
			localQuotaState = "available"
			if localError != "" {
				localQuotaState = "stale"
			}
		}
	}
	writeJSON(w, map[string]any{"items": []map[string]any{{
		"id": x.InstanceID, "adapter": x.Adapter, "endpoint": redactEndpoint(x.Endpoint),
		"secret_ref": x.SecretRef, "configured": i.FreeLLMAPI.Configured(), "recognized": x.Recognized,
		"authorized": x.Authorized, "route_enabled": x.RouteEnabled, "connected": x.Connected,
		"catalog_fresh": x.CatalogFresh, "usable": i.FreeLLMAPI.RouteEnabled(), "status": x.Status,
		"release": x.Release, "model_count": x.ModelCount, "available_models": x.AvailableModels,
		"ready_upstreams": x.ReadyUpstreams, "summary_supported": x.SummarySupported,
		"last_probe": x.LastProbe, "last_sync": x.LastSync, "last_quota": x.LastQuota,
		"last_error": x.LastError, "generation": x.Generation,
		"local_quota_state": localQuotaState, "local_quota_checked_at": localChecked, "local_quota_error": localError,
		"local_quota_source": "freellmapi_sqlite_readonly", "local_quota": local, "local_quota_stale": local != nil && localError != "",
	}}})
}

func redactEndpoint(endpoint string) string {
	if endpoint == "" {
		return ""
	}
	return endpoint
}

func (i *IntegrationRuntime) LocalQuota(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if i == nil || i.FreeLLMAPI == nil || !i.FreeLLMAPI.Authorized() {
		http.Error(w, "integration not authorized", http.StatusForbidden)
		return
	}
	report, checkedAt, errText := i.FreeLLMAPI.LocalQuotaState()
	writeJSON(w, map[string]any{"available": report != nil, "source": "freellmapi_sqlite_readonly", "checked_at": checkedAt, "error": errText, "report": report})
}

func (i *IntegrationRuntime) LocalQuotaRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if i == nil || i.FreeLLMAPI == nil || !i.FreeLLMAPI.Authorized() {
		http.Error(w, "integration not authorized", http.StatusForbidden)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	report, err := i.FreeLLMAPI.RefreshLocalQuota(ctx)
	if err != nil {
		i.audit("integration_local_quota_refresh", "error")
		writeJSON(w, map[string]any{"ok": false, "available": false, "error": safeLocalQuotaAPIError(err)})
		return
	}
	i.audit("integration_local_quota_refresh", "ok")
	writeJSON(w, map[string]any{"ok": true, "available": true, "source": "freellmapi_sqlite_readonly", "report": report})
}

func (i *IntegrationRuntime) ApplyLocalQuota() {
	if i == nil || i.FreeLLMAPI == nil || !i.FreeLLMAPI.Authorized() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, _ = i.FreeLLMAPI.RefreshLocalQuota(ctx)
}

func safeLocalQuotaAPIError(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if text == "freellmapi integration not authorized" {
		return "not_authorized"
	}
	if text == "freellmapi local database is not configured" {
		return "local_database_not_configured"
	}
	return "local_database_unavailable"
}

func (i *IntegrationRuntime) Probe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if i == nil || i.FreeLLMAPI == nil {
		http.Error(w, "integration unavailable", http.StatusNotFound)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	report, err := i.FreeLLMAPI.Probe(ctx)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "probe_failed", "state": report.State})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "state": report.State, "models": len(report.Models), "providers": report.Providers, "summary": report.Summary, "summary_status": report.SummaryStatus})
}

func (i *IntegrationRuntime) Authorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if i == nil || i.FreeLLMAPI == nil {
		http.Error(w, "integration unavailable", http.StatusNotFound)
		return
	}
	var body struct {
		RouteEnabled *bool `json:"route_enabled"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body)
	route := body.RouteEnabled != nil && *body.RouteEnabled
	state := i.FreeLLMAPI.Authorize(route)
	i.ApplyLocalQuota()
	i.ApplyStateToCatalog()
	state = i.FreeLLMAPI.State()
	i.audit("integration_authorize", "ok")
	writeJSON(w, map[string]any{"ok": true, "state": state})
}

func (i *IntegrationRuntime) Revoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if i == nil || i.FreeLLMAPI == nil {
		http.Error(w, "integration unavailable", http.StatusNotFound)
		return
	}
	state := i.FreeLLMAPI.Revoke()
	i.audit("integration_revoke", "ok")
	if i.Catalog != nil {
		i.ApplyStateToCatalog()
	}
	writeJSON(w, map[string]any{"ok": true, "state": state})
}

func (i *IntegrationRuntime) Route(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if i == nil || i.FreeLLMAPI == nil {
		http.Error(w, "integration unavailable", http.StatusNotFound)
		return
	}
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil || body.Enabled == nil {
		http.Error(w, "enabled is required", http.StatusBadRequest)
		return
	}
	state := i.FreeLLMAPI.SetRoute(*body.Enabled)
	i.audit("integration_route_enable", map[bool]string{true: "enabled", false: "disabled"}[*body.Enabled])
	if !*body.Enabled {
		i.ApplyStateToCatalog()
	}
	writeJSON(w, map[string]any{"ok": true, "state": state})
}

func (i *IntegrationRuntime) Sync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if i == nil || i.FreeLLMAPI == nil || !i.FreeLLMAPI.Authorized() {
		http.Error(w, "integration not authorized", http.StatusForbidden)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	report, err := i.FreeLLMAPI.Probe(ctx)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "probe_failed", "state": report.State})
		return
	}
	if i.Catalog != nil {
		result := i.Catalog.SyncRaw(integration.FreeLLMAPIID, report.Models)
		if result.ErrorType != "" {
			writeJSON(w, map[string]any{"ok": false, "error": string(result.ErrorType), "message": result.Message})
			return
		}
		i.FreeLLMAPI.MarkSynced(len(report.Models))
	}
	if report.Summary != nil && i.Quota != nil {
		i.FreeLLMAPI.ApplySummary(report.Summary, i.Quota)
	}
	_, _ = i.FreeLLMAPI.RefreshLocalQuota(ctx)
	i.audit("integration_sync", "ok")
	writeJSON(w, map[string]any{"ok": true, "state": i.FreeLLMAPI.State(), "models": len(report.Models), "summary": report.Summary})
}

func (i *IntegrationRuntime) QuotaRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if i == nil || i.FreeLLMAPI == nil || !i.FreeLLMAPI.Authorized() {
		http.Error(w, "integration not authorized", http.StatusForbidden)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	report, err := i.FreeLLMAPI.Probe(ctx)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "probe_failed"})
		return
	}
	if report.Summary == nil {
		writeJSON(w, map[string]any{"ok": false, "available": false, "message": "FreeLLMAPI did not expose a quota summary"})
		return
	}
	if i.Quota != nil {
		i.FreeLLMAPI.ApplySummary(report.Summary, i.Quota)
	}
	i.audit("integration_quota_refresh", "ok")
	writeJSON(w, map[string]any{"ok": true, "available": len(report.Summary.QuotaPools) > 0, "summary": report.Summary})
}

func (i *IntegrationRuntime) ApplyStateToCatalog() {
	if i == nil || i.FreeLLMAPI == nil || i.Catalog == nil {
		return
	}
	state := i.FreeLLMAPI.State()
	allow := i.Catalog.AllowSnapshot()
	enabled := state.Authorized && state.RouteEnabled
	for _, model := range i.Catalog.Snapshot().Models {
		if model.Provider != integration.FreeLLMAPIID {
			continue
		}
		approved := allow[model.Provider+"/"+model.ID] || catalog.IsAutoApprovedProvider(model.Provider)
		allowed := enabled && approved && model.Status == catalog.Active
		i.Catalog.UpdateProviderModel(model.ID, model.Provider, func(x *catalog.Model) { x.AutoRoutable = allowed })
	}
}
func (i *IntegrationRuntime) Background(ctx context.Context, interval time.Duration) {
	if i == nil || i.FreeLLMAPI == nil || interval <= 0 {
		return
	}
	go func() {
		run := func() {
			if !i.FreeLLMAPI.Authorized() {
				return
			}
			requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			report, err := i.FreeLLMAPI.Probe(requestCtx)
			if err != nil || i.Catalog == nil {
				if err != nil {
					i.audit("integration_background_probe", "error")
				}
				return
			}
			_, _ = i.FreeLLMAPI.RefreshLocalQuota(requestCtx)
			result := i.Catalog.SyncRaw(integration.FreeLLMAPIID, report.Models)
			if result.ErrorType == "" {
				i.FreeLLMAPI.MarkSynced(len(report.Models))
				if report.Summary != nil && i.Quota != nil {
					i.FreeLLMAPI.ApplySummary(report.Summary, i.Quota)
				}
				_, _ = i.FreeLLMAPI.RefreshLocalQuota(requestCtx)
				i.audit("integration_background_sync", "ok")
			} else {
				i.audit("integration_background_sync", "error")
			}
		}
		run()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}
