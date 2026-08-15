package api

import (
	"encoding/json"
	"net/http"
	"time"
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
		ranked := service.RankedCandidates(raw)
		eligible := map[string]routing.RankedCandidate{}
		for _, candidate := range ranked {
			eligible[candidate.Provider+"/"+candidate.Model] = candidate
		}
		items := make([]explainCandidate, 0)
		rows := make([]map[string]any, 0)
		excluded := 0
		for _, model := range serviceModels(service) {
			if model.Status != catalog.Active || !model.AutoRoutable || !service.HasProvider(model.Provider) {
				continue
			}
			key := model.Provider + "/" + model.ID
			if candidate, ok := eligible[key]; ok {
				item := explainCandidate{Provider: model.Provider, Model: model.ID, Eligible: true, Reasons: candidate.Reasons, ScoreBreakdown: candidate.ScoreBreakdown, TotalScore: candidate.Score}
				items = append(items, item)
				rows = append(rows, map[string]any{"provider": model.Provider, "model": model.ID, "status": model.Status, "auto_routable": true, "rule_score": candidate.Score, "knowledge_applied": candidate.ScoreBreakdown["knowledge_bonus"] > 0, "reasons": candidate.Reasons, "score_breakdown": candidate.ScoreBreakdown})
				continue
			}
			reasons := []string{}
			if need != "general" && !supportsNeed(model, need) {
				reasons = append(reasons, "capability_mismatch")
			}
			if !service.RouteAvailable(model.Provider, model.ID, time.Now()) {
				reasons = append(reasons, "cooldown_or_recovery_probe")
			}
			if len(reasons) == 0 {
				continue
			}
			excluded++
			breakdown := map[string]int{"base_score": model.Score, "knowledge_bonus": service.KnowledgeBonus(raw, model), "learning_bonus": 0, "health_adjustment": 0}
			item := explainCandidate{Provider: model.Provider, Model: model.ID, Eligible: false, Reasons: reasons, ScoreBreakdown: breakdown, TotalScore: model.Score + breakdown["knowledge_bonus"]}
			items = append(items, item)
			rows = append(rows, map[string]any{"provider": model.Provider, "model": model.ID, "status": model.Status, "auto_routable": false, "rule_score": item.TotalScore, "knowledge_applied": breakdown["knowledge_bonus"] > 0, "reasons": reasons, "score_breakdown": breakdown})
		}
		selected := ""
		if len(ranked) > 0 {
			selected = ranked[0].Model
		}
		writeJSON(w, map[string]any{"request": body, "need": need, "selected": selected, "excluded": excluded, "candidates": items, "items": rows, "health": service.HealthSnapshot()})
	}
}
func serviceModels(service *routing.Service) []catalog.Model { return service.SnapshotModels() }
func supportsNeed(model catalog.Model, need string) bool     { return capability.Supports(model, need) }
