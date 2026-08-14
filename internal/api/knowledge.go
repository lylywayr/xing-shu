package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"xing-shu/internal/catalog"
	"xing-shu/internal/runtime"
)

func KnowledgeView(k *runtime.KnowledgeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if k == nil {
			writeJSON(w, map[string]any{"items": []any{}})
			return
		}
		k.Expire(time.Now())
		all := k.All()
		stats := map[string]int{"proposed": 0, "validated": 0, "trusted": 0, "rejected": 0, "expired": 0}
		for _, x := range all {
			stats[string(x.Status)]++
		}
		status := strings.TrimSpace(r.URL.Query().Get("status"))
		items := make([]runtime.Knowledge, 0, len(all))
		for _, x := range all {
			if status == "" || string(x.Status) == status {
				items = append(items, x)
			}
		}
		writeJSON(w, map[string]any{"items": items, "stats": stats, "status": status, "total": len(items)})
	}
}
func BuildKnowledge(rt *Runtime) int {
	if rt == nil || rt.Knowledge == nil || rt.Reviews == nil {
		return 0
	}
	n := 0
	for _, x := range rt.Reviews.All() {
		if x.ReviewStatus != runtime.StatusReviewed || x.RecommendedModel == "" || x.Confidence <= 0 {
			continue
		}
		now := time.Now()
		id := runtime.Signature(x.Features) + "|" + x.RecommendedModel
		rt.Knowledge.Upsert(runtime.Knowledge{ID: id, Signature: runtime.Signature(x.Features), Features: x.Features, ActualModel: x.Model, ActualProvider: x.Provider, RecommendedModel: x.RecommendedModel, MinimumTier: x.MinimumTier, Confidence: x.Confidence, Samples: 1, Status: runtime.KnowledgeProposed, SourceReviewIDs: []string{x.ID}, CreatedAt: now, UpdatedAt: now})
		n++
	}
	return n
}
func KnowledgeBuild(rt *Runtime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || rt == nil || rt.Knowledge == nil || rt.Reviews == nil {
			http.Error(w, "knowledge unavailable", 503)
			return
		}
		n := BuildKnowledge(rt)
		writeJSON(w, map[string]any{"created": n, "status": "proposed"})
	}
}
func KnowledgeDecision(rt *Runtime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || rt == nil || rt.Knowledge == nil {
			http.Error(w, "knowledge unavailable", 503)
			return
		}
		var x struct {
			ID     string                  `json:"id"`
			Status runtime.KnowledgeStatus `json:"status"`
			Note   string                  `json:"note"`
			Cause  runtime.FailureCause    `json:"failure_cause"`
		}
		if json.NewDecoder(r.Body).Decode(&x) != nil || x.ID == "" {
			http.Error(w, "id required", 400)
			return
		}
		if x.Status != runtime.KnowledgeValidated && x.Status != runtime.KnowledgeTrusted && x.Status != runtime.KnowledgeRejected {
			http.Error(w, "invalid status", 400)
			return
		}
		var current runtime.Knowledge
		found := false
		for _, item := range rt.Knowledge.All() {
			if item.ID == x.ID {
				current, found = item, true
				break
			}
		}
		if !found {
			http.Error(w, "knowledge not found", http.StatusNotFound)
			return
		}
		if !knowledgeTransitionAllowed(current.Status, x.Status) {
			http.Error(w, "invalid knowledge transition", http.StatusConflict)
			return
		}
		if !rt.Knowledge.ChangeStatus(x.ID, x.Status, x.Note, x.Cause) {
			http.Error(w, "knowledge transition could not be recorded", http.StatusConflict)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "id": x.ID, "status": x.Status})
	}
}
func knowledgeTransitionAllowed(from, to runtime.KnowledgeStatus) bool {
	if from == to {
		return false
	}
	switch from {
	case runtime.KnowledgeProposed:
		return to == runtime.KnowledgeValidated || to == runtime.KnowledgeRejected
	case runtime.KnowledgeValidated:
		return to == runtime.KnowledgeTrusted || to == runtime.KnowledgeRejected
	case runtime.KnowledgeTrusted:
		return to == runtime.KnowledgeRejected
	case runtime.KnowledgeExpired, runtime.KnowledgeRejected:
		return false
	}
	return false
}
func KnowledgeRestore(rt *Runtime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || rt == nil || rt.Knowledge == nil {
			http.Error(w, "knowledge unavailable", http.StatusServiceUnavailable)
			return
		}
		var x struct {
			ID     string                  `json:"id"`
			Status runtime.KnowledgeStatus `json:"status"`
			Note   string                  `json:"note"`
		}
		if json.NewDecoder(r.Body).Decode(&x) != nil || x.ID == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		if x.Status != runtime.KnowledgeValidated && x.Status != runtime.KnowledgeTrusted {
			http.Error(w, "invalid restore status", http.StatusBadRequest)
			return
		}
		if !rt.Knowledge.Restore(x.ID, x.Status, x.Note) {
			http.Error(w, "knowledge must be expired or rejected", http.StatusConflict)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "id": x.ID, "status": x.Status})
	}
}
func KnowledgeValidate(rt *Runtime, manager *catalog.Manager, ops *OpsState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || rt == nil || rt.Knowledge == nil {
			http.Error(w, "knowledge unavailable", 503)
			return
		}
		var x struct {
			ID           string               `json:"id"`
			ValidationID string               `json:"validation_id"`
			Success      bool                 `json:"success"`
			Cause        runtime.FailureCause `json:"failure_cause"`
			Note         string               `json:"note"`
		}
		if json.NewDecoder(r.Body).Decode(&x) != nil || x.ID == "" {
			http.Error(w, "id required", 400)
			return
		}
		ok := false
		for _, k := range rt.Knowledge.All() {
			if k.ID == x.ID {
				ok = true
				break
			}
		}
		if !ok {
			http.Error(w, "knowledge not found", 404)
			return
		}
		var candidate runtime.Knowledge
		for _, k := range rt.Knowledge.All() {
			if k.ID == x.ID {
				candidate = k
				break
			}
		}
		if !modelAvailable(manager, ops, candidate.RecommendedModel) {
			http.Error(w, "recommended model unavailable", 409)
			return
		}
		if x.ValidationID == "" {
			http.Error(w, "validation_id required", 400)
			return
		}
		if !x.Success && x.Cause == "" {
			x.Cause = runtime.CauseUnknown
		}
		if !rt.Knowledge.RecordValidation(x.ID, x.ValidationID, x.Success, x.Cause, x.Note) {
			http.Error(w, "validation could not be recorded", 409)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "shadow_execution": "not performed", "message": "validation result recorded; no automatic replay or Auto routing change"})
	}
}
func modelAvailable(manager *catalog.Manager, ops *OpsState, id string) bool {
	if manager == nil || id == "" {
		return false
	}
	for _, m := range manager.Snapshot().Models {
		if m.ID == id && m.Status == catalog.Active && m.AutoRoutable && (ops == nil || !ops.IsDisabled(m.Provider)) {
			return true
		}
	}
	return false
}
func KnowledgeModels(manager *catalog.Manager, ops *OpsState) []runtime.ReviewModel {
	out := []runtime.ReviewModel{}
	if manager == nil {
		return out
	}
	for _, m := range manager.Snapshot().Models {
		if m.Status == catalog.Active && m.AutoRoutable && (ops == nil || !ops.IsDisabled(m.Provider)) {
			out = append(out, runtime.ReviewModel{ID: m.ID, Provider: m.Provider, Status: string(m.Status), AutoRoutable: true, Capabilities: m.Capabilities, Tools: m.Tools, Vision: m.Vision, StructuredOutput: m.StructuredOutput, ContextWindow: m.ContextWindow})
		}
	}
	return out
}
