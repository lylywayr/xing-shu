package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"
	"xing-shu/internal/catalog"
	"xing-shu/internal/runtime"
)

func ShadowBatchesView(batches *runtime.ShadowBatches) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if batches == nil {
			writeJSON(w, map[string]any{"items": []any{}, "total": 0, "limit": queryLimit(r, 50, 100), "offset": queryOffset(r)})
			return
		}
		status, knowledgeID := strings.TrimSpace(r.URL.Query().Get("status")), strings.TrimSpace(r.URL.Query().Get("knowledge_id"))
		all := batches.All()
		filtered := make([]runtime.ShadowBatch, 0, len(all))
		for _, item := range all {
			if (status == "" || string(item.Status) == status) && (knowledgeID == "" || item.KnowledgeID == knowledgeID) {
				filtered = append(filtered, item)
			}
		}
		limit, offset := queryLimit(r, 50, 100), queryOffset(r)
		if offset > len(filtered) {
			offset = len(filtered)
		}
		end := offset + limit
		if end > len(filtered) {
			end = len(filtered)
		}
		writeJSON(w, map[string]any{"items": filtered[offset:end], "total": len(filtered), "limit": limit, "offset": offset, "status": status, "knowledge_id": knowledgeID})
	}
}

func ShadowBatchDetail(batches *runtime.ShadowBatches) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if batches == nil {
			http.Error(w, "shadow batches unavailable", http.StatusServiceUnavailable)
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		item, ok := batches.Get(id)
		if !ok {
			http.Error(w, "batch not found", http.StatusNotFound)
			return
		}
		writeJSON(w, item)
	}
}

func ShadowBatchStop(batches *runtime.ShadowBatches) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || batches == nil {
			http.Error(w, "shadow batches unavailable", http.StatusServiceUnavailable)
			return
		}
		var q struct {
			BatchID string `json:"batch_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		if q.BatchID == "" {
			q.BatchID = r.URL.Query().Get("batch_id")
		}
		if q.BatchID == "" {
			http.Error(w, "batch_id required", http.StatusBadRequest)
			return
		}
		if !batches.Stop(q.BatchID) {
			http.Error(w, "batch not found", http.StatusNotFound)
			return
		}
		item, _ := batches.Get(q.BatchID)
		writeJSON(w, map[string]any{"ok": true, "batch": item})
	}
}

type shadowBatchRequest struct {
	KnowledgeID    string   `json:"knowledge_id"`
	ReviewIDs      []string `json:"review_ids"`
	CandidateModel string   `json:"candidate_model"`
}

func ShadowBatchStart(rt *Runtime, manager *catalog.Manager, ops *OpsState, batches *runtime.ShadowBatches) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || rt == nil || rt.Knowledge == nil || rt.Reviews == nil || batches == nil {
			http.Error(w, "shadow unavailable", http.StatusServiceUnavailable)
			return
		}
		var q shadowBatchRequest
		if json.NewDecoder(r.Body).Decode(&q) != nil || q.KnowledgeID == "" || len(q.ReviewIDs) == 0 || strings.TrimSpace(q.CandidateModel) == "" {
			http.Error(w, "knowledge_id, review_ids and candidate_model required", http.StatusBadRequest)
			return
		}
		knowledge, ok := findKnowledge(rt.Knowledge, q.KnowledgeID)
		if !ok {
			http.Error(w, "knowledge not found", http.StatusNotFound)
			return
		}
		if knowledge.Status != runtime.KnowledgeTrusted {
			http.Error(w, "knowledge must be trusted", http.StatusConflict)
			return
		}
		candidate := q.CandidateModel
		if candidate == "" {
			candidate = knowledge.RecommendedModel
		}
		if !modelAvailable(manager, ops, candidate) {
			http.Error(w, "candidate unavailable", http.StatusConflict)
			return
		}
		valid := make([]string, 0, len(q.ReviewIDs))
		for _, id := range q.ReviewIDs {
			rev, exists := rt.Reviews.Find(id)
			if !exists || !rev.Replayable || len(rev.TaskPackage) == 0 {
				http.Error(w, "review source unavailable: "+id, http.StatusConflict)
				return
			}
			valid = append(valid, id)
		}
		now := time.Now().UTC()
		id := "shadow-batch-" + now.Format("20060102150405.000000000")
		item := runtime.ShadowBatch{ID: id, KnowledgeID: q.KnowledgeID, ReviewIDs: valid, CandidateModel: candidate, Status: runtime.ShadowBatchPending, Total: len(valid), CreatedAt: now, UpdatedAt: now}
		batches.Put(item)
		batchCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		go func() { defer cancel(); runShadowBatch(batchCtx, rt, manager, ops, batches, item) }()
		writeJSON(w, map[string]any{"ok": true, "batch_id": id, "status": item.Status, "total": item.Total})
	}
}

func runShadowBatch(ctx context.Context, rt *Runtime, manager *catalog.Manager, ops *OpsState, batches *runtime.ShadowBatches, batch runtime.ShadowBatch) {
	batches.Apply(batch.ID, func(x *runtime.ShadowBatch) { x.Status = runtime.ShadowBatchRunning })
	for _, reviewID := range batch.ReviewIDs {
		select {
		case <-ctx.Done():
			batches.Apply(batch.ID, func(x *runtime.ShadowBatch) {
				if x.Status == runtime.ShadowBatchRunning {
					x.Status = runtime.ShadowBatchStopped
				}
			})
			return
		default:
		}
		current, ok := batches.Get(batch.ID)
		if !ok || current.Status == runtime.ShadowBatchStopped {
			return
		}
		body, _ := json.Marshal(map[string]string{"knowledge_id": batch.KnowledgeID, "review_id": reviewID, "candidate_model": batch.CandidateModel, "batch_id": batch.ID})
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/admin/shadow/run", strings.NewReader(string(body)))
		rr := httptest.NewRecorder()
		ShadowRun(rt, manager, ops).ServeHTTP(rr, req)
		batches.Apply(batch.ID, func(x *runtime.ShadowBatch) {
			if x.Status == runtime.ShadowBatchStopped {
				return
			}
			x.Completed++
			if rr.Code >= 200 && rr.Code < 300 {
				x.Succeeded++
			} else {
				x.Failed++
				x.LastError = strings.TrimSpace(rr.Body.String())
			}
		})
	}
	batches.Apply(batch.ID, func(x *runtime.ShadowBatch) {
		if x.Status == runtime.ShadowBatchRunning {
			if x.Failed > 0 {
				x.Status = runtime.ShadowBatchFailed
			} else {
				x.Status = runtime.ShadowBatchCompleted
			}
		}
	})
}

func findKnowledge(store *runtime.KnowledgeStore, id string) (runtime.Knowledge, bool) {
	for _, item := range store.All() {
		if item.ID == id {
			return item, true
		}
	}
	return runtime.Knowledge{}, false
}
func shadowBatchInt(value string, fallback int) int {
	x, e := strconv.Atoi(value)
	if e != nil {
		return fallback
	}
	return x
}

var _ catalog.Model
