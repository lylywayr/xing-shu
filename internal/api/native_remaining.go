package api

import (
	"net/http"
	"xing-shu/internal/catalog"
	"xing-shu/internal/governance"
	"xing-shu/internal/provider"
	"xing-shu/internal/runtime"
)

func NativeRoutingWithKnowledge(m *catalog.Manager, k *runtime.KnowledgeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items := []map[string]any{}
		for _, x := range m.Snapshot().Models {
			items = append(items, map[string]any{"provider": x.Provider, "model": x.ID, "capabilities": x.Capabilities, "auto_routable": x.AutoRoutable, "status": x.Status, "rule_score": x.Score, "evidence": x.CapabilityEvidence, "knowledge_applied": knowledgeForModel(k, x.ID), "knowledge_note": knowledgeNote(k, x.ID)})
		}
		writeJSON(w, map[string]any{"items": items})
	}
}
func knowledgeForModel(k *runtime.KnowledgeStore, id string) bool {
	if k == nil {
		return false
	}
	for _, x := range k.All() {
		if x.RecommendedModel == id && x.Status == runtime.KnowledgeTrusted {
			return true
		}
	}
	return false
}
func knowledgeNote(k *runtime.KnowledgeStore, id string) string {
	if knowledgeForModel(k, id) {
		return "trusted knowledge candidate; actual match depends on task signature"
	}
	return "no trusted knowledge"
}
func NativeConsistency(m *catalog.Manager, g []governance.Record) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := m.Snapshot()
		reconciled := reconcileGovernance(c, g)
		known := map[string]bool{}
		for _, x := range reconciled {
			known[x.Key] = true
		}
		missing := 0
		for _, x := range c.Models {
			if !known[x.Provider+"/"+x.ID] {
				missing++
			}
		}
		writeJSON(w, map[string]any{"catalog": len(c.Models), "governance": len(reconciled), "missing_governance": missing, "ok": missing == 0})
	}
}
func NativeRegression(m *catalog.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bad := []string{}
		for _, x := range m.Snapshot().Models {
			if x.AutoRoutable && x.Status != catalog.Active {
				bad = append(bad, x.Provider+"/"+x.ID)
			}
			if catalog.IsMeta(x.ID) && x.AutoRoutable {
				bad = append(bad, "meta:"+x.ID)
			}
		}
		writeJSON(w, map[string]any{"ok": len(bad) == 0, "violations": bad})
	}
}
func NativeProfiles(m *catalog.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items := []map[string]any{}
		for _, x := range m.Snapshot().Models {
			items = append(items, map[string]any{"provider": x.Provider, "model": x.ID, "capabilities": x.Capabilities, "tools": x.Tools, "vision": x.Vision, "context_window": x.ContextWindow, "evidence": x.CapabilityEvidence})
		}
		writeJSON(w, map[string]any{"items": items})
	}
}
func NativeAccounts(configs map[string]provider.Config, manager *catalog.Manager, ops *OpsState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items := make([]map[string]any, 0, len(configs))
		for id, cfg := range configs {
			models := 0
			if manager != nil {
				for _, m := range manager.Snapshot().Models {
					if m.Provider == id {
						models++
					}
				}
			}
			disabled := ops != nil && ops.IsDisabled(id)
			status := "configured"
			if disabled {
				status = "disabled"
			}
			items = append(items, map[string]any{"provider": id, "kind": cfg.Kind, "configured": cfg.BaseURL != "", "status": status, "models": models, "api_key_configured": cfg.APIKey != "", "base_url_configured": cfg.BaseURL != ""})
		}
		writeJSON(w, map[string]any{"configured": len(items) > 0, "accounts": items})
	}
}
