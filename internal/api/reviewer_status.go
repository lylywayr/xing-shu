package api

import "xing-shu/internal/catalog"

func reviewerStatus(rt *Runtime, manager *catalog.Manager, ops *OpsState) map[string]any {
	status := map[string]any{}
	if rt == nil || rt.ReviewerSelection == nil {
		return status
	}
	cfg := rt.ReviewerSelection.Get()
	status["model"] = cfg.Model
	status["provider"] = cfg.Provider
	status["selected_at"] = cfg.SelectedAt
	status["last_checked_at"] = cfg.LastCheckedAt
	status["consecutive_failures"] = cfg.ConsecutiveFailures
	status["last_error"] = cfg.LastError
	if cfg.Model == "" {
		status["available"] = false
		status["state"] = "not_selected"
		return status
	}
	if ops != nil && ops.IsDisabled(cfg.Provider) {
		status["available"] = false
		status["state"] = "provider_disabled"
		return status
	}
	status["available"] = validReviewerModel(manager, ops, cfg.Model, cfg.Provider)
	if status["available"] == true {
		status["state"] = "ready"
	} else {
		status["state"] = "capability_unknown_or_unavailable"
	}
	return status
}
