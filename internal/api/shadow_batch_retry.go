package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"xing-shu/internal/catalog"
	"xing-shu/internal/runtime"
)

func ShadowBatchRetry(rt *Runtime, manager *catalog.Manager, ops *OpsState, batches *runtime.ShadowBatches) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || rt == nil || batches == nil {
			http.Error(w, "shadow unavailable", http.StatusServiceUnavailable)
			return
		}
		var q struct {
			BatchID string `json:"batch_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		if q.BatchID == "" {
			q.BatchID = r.URL.Query().Get("batch_id")
		}
		if strings.TrimSpace(q.BatchID) == "" {
			http.Error(w, "batch_id required", http.StatusBadRequest)
			return
		}
		item, ok := batches.Get(q.BatchID)
		if !ok {
			http.Error(w, "batch not found", http.StatusNotFound)
			return
		}
		if item.Status != runtime.ShadowBatchFailed && item.Status != runtime.ShadowBatchStopped {
			http.Error(w, "batch is not retryable", http.StatusConflict)
			return
		}
		if !batches.Retry(q.BatchID) {
			http.Error(w, "batch is not retryable", http.StatusConflict)
			return
		}
		item, _ = batches.Get(q.BatchID)
		batchCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		go func() { defer cancel(); runShadowBatch(batchCtx, rt, manager, ops, batches, item) }()
		writeJSON(w, map[string]any{"ok": true, "batch": item})
	}
}
