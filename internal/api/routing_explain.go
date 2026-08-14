package api

import (
	"encoding/json"
	"net/http"
	"xing-shu/internal/capability"
	"xing-shu/internal/catalog"
	"xing-shu/internal/routing"
)

type explainCandidate struct {
	Provider       string         `json:"provider"`
	Model          string         `json:"model"`
	Eligible       bool           `json:"eligible"`
	Reasons        []string       `json:"reasons"`
	ScoreBreakdown map[string]int `json:"score_breakdown"`
	TotalScore     int            `json:"total_score"`
}

func RoutingExplain(service *routing.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body := map[string]any{}
		if r.Method == http.MethodPost && json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024)).Decode(&body) != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		raw, err := json.Marshal(body)
		if err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		need := routing.Needs(raw)
		selected := service.SelectAuto(raw)
		items := make([]explainCandidate, 0)
		excluded := 0
		for _, model := range serviceModels(service) {
			if model.Status != "active" {
				excluded++
				continue
			}
			reasons := []string{}
			base := model.Score
			providerAdjustment := 0
			knowledgeBonus := service.KnowledgeBonus(raw, model)
			if !model.AutoRoutable {
				reasons = append(reasons, "not_auto_routable")
			}
			if !service.HasProvider(model.Provider) {
				reasons = append(reasons, "provider_unavailable")
			}
			if service.QuotaExhausted(model.Provider) {
				reasons = append(reasons, "quota_exhausted")
			}
			if need != "general" && !supportsNeed(model, need) {
				reasons = append(reasons, "capability_mismatch")
			}
			eligible := len(reasons) == 0
			if !eligible {
				excluded++
			}
			items = append(items, explainCandidate{Provider: model.Provider, Model: model.ID, Eligible: eligible, Reasons: reasons, ScoreBreakdown: map[string]int{"base_score": base, "provider_adjustment": providerAdjustment, "knowledge_bonus": knowledgeBonus}, TotalScore: base + providerAdjustment + knowledgeBonus})
		}
		rows := make([]map[string]any, 0, len(items))
		for _, item := range items {
			rows = append(rows, map[string]any{"provider": item.Provider, "model": item.Model, "status": "active", "auto_routable": item.Eligible, "rule_score": item.TotalScore, "knowledge_applied": item.ScoreBreakdown["knowledge_bonus"] > 0, "reasons": item.Reasons, "score_breakdown": item.ScoreBreakdown})
		}
		writeJSON(w, map[string]any{"request": body, "need": need, "selected": selected, "excluded": excluded, "candidates": items, "items": rows})
	}
}

func serviceModels(service *routing.Service) []catalog.Model { return service.SnapshotModels() }
func supportsNeed(model catalog.Model, need string) bool {
	return capability.Supports(model, need)
}
