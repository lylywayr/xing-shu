package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"xing-shu/internal/runtime"
)

func ReviewDetailView(reviews *runtime.Reviews, rawLimit int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || reviews == nil {
			http.Error(w, "reviews unavailable", http.StatusServiceUnavailable)
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		item, ok := reviews.Find(id)
		if !ok {
			http.Error(w, "review not found", http.StatusNotFound)
			return
		}
		result := reviewSummary(item)
		result["task_package_size"] = len(item.TaskPackage)
		result["response_package_size"] = len(item.ResponsePackage)
		if r.URL.Query().Get("raw") == "true" {
			if rawLimit <= 0 {
				rawLimit = 64 * 1024
			}
			result["task_package"] = cappedString(item.TaskPackage, rawLimit)
			result["response_package"] = cappedString(item.ResponsePackage, rawLimit)
		} else if r.URL.Query().Get("raw") != "" {
			http.Error(w, "raw must be true when provided", http.StatusBadRequest)
			return
		}
		writeJSON(w, result)
	}
}

func cappedString(data []byte, limit int) string {
	if len(data) <= limit {
		return string(data)
	}
	suffix := fmt.Sprintf("…[truncated %d bytes]", len(data)-limit)
	if len(suffix) >= limit {
		return suffix[:limit]
	}
	return string(data[:limit-len(suffix)]) + suffix
}

type reviewBatchAggregate struct {
	BatchID    string   `json:"batch_id"`
	Total      int      `json:"total"`
	Pending    int      `json:"pending"`
	Processing int      `json:"processing"`
	Reviewed   int      `json:"reviewed"`
	Failed     int      `json:"failed"`
	Skipped    int      `json:"skipped"`
	Models     []string `json:"models"`
	Providers  []string `json:"providers"`
	LastError  string   `json:"last_error,omitempty"`
}

func ReviewBatchesView(reviews *runtime.Reviews) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || reviews == nil {
			http.Error(w, "reviews unavailable", http.StatusServiceUnavailable)
			return
		}
		groups := map[string]*reviewBatchAggregate{}
		for _, item := range reviews.All() {
			batchID := item.BatchID
			if batchID == "" {
				batchID = "unbatched"
			}
			group := groups[batchID]
			if group == nil {
				group = &reviewBatchAggregate{BatchID: batchID}
				groups[batchID] = group
			}
			group.Total++
			switch item.ReviewStatus {
			case runtime.StatusPending:
				group.Pending++
			case runtime.StatusProcessing:
				group.Processing++
			case runtime.StatusReviewed:
				group.Reviewed++
			case runtime.StatusFailed:
				group.Failed++
			case runtime.StatusSkipped:
				group.Skipped++
			}
			if item.ReviewError != "" {
				group.LastError = item.ReviewError
			}
			group.Models = appendUnique(group.Models, item.Model)
			group.Providers = appendUnique(group.Providers, item.Provider)
		}
		items := make([]*reviewBatchAggregate, 0, len(groups))
		for _, item := range groups {
			items = append(items, item)
		}
		writeJSON(w, map[string]any{"items": items, "total": len(items), "limit": queryLimit(r, 50, 100), "offset": queryOffset(r)})
	}
}
func appendUnique(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}
func reviewDetailRawLimit(r *http.Request) int {
	value, err := strconv.Atoi(r.URL.Query().Get("raw_limit"))
	if err != nil || value <= 0 {
		return 64 * 1024
	}
	if value > 256*1024 {
		return 256 * 1024
	}
	return value
}

var _ = json.Valid
