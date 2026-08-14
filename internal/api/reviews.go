package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"
	"xing-shu/internal/catalog"
	"xing-shu/internal/provider"
	"xing-shu/internal/runtime"
)

func ReviewsView(v *runtime.Reviews) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if v == nil {
			writeJSON(w, map[string]any{"items": []any{}, "total": 0, "limit": 50, "offset": 0})
			return
		}
		limit := queryLimit(r, 50, 200)
		offset := queryOffset(r)
		all := v.All()
		if offset > len(all) {
			offset = len(all)
		}
		end := offset + limit
		if end > len(all) {
			end = len(all)
		}
		items := make([]map[string]any, 0, end-offset)
		for _, item := range all[offset:end] {
			items = append(items, reviewSummary(item))
		}
		writeJSON(w, map[string]any{"items": items, "total": len(all), "limit": limit, "offset": offset})
	}
}

func queryLimit(r *http.Request, fallback, maximum int) int {
	value, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || value <= 0 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}

func queryOffset(r *http.Request) int {
	value, err := strconv.Atoi(r.URL.Query().Get("offset"))
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func reviewSummary(item runtime.Review) map[string]any {
	return map[string]any{
		"id": item.ID, "recorded_at": item.RecordedAt, "model": item.Model, "provider": item.Provider,
		"status": item.Status, "latency_ms": item.LatencyMS, "stream": item.Stream, "tools": item.Tools,
		"features": item.Features, "review_status": item.ReviewStatus, "batch_id": item.BatchID,
		"attempts": item.Attempts, "reviewer_model": item.ReviewerModel, "reviewed_at": item.ReviewedAt,
		"recommended_model": item.RecommendedModel, "minimum_tier": item.MinimumTier,
		"overqualified": item.Overqualified, "confidence": item.Confidence, "conclusion": item.Conclusion,
		"review_error": item.ReviewError, "replayable": item.Replayable, "replay_reason": item.ReplayReason,
		"response_captured": item.ResponseCaptured, "task_package_encrypted": item.TaskPackageEncrypted,
	}
}
func reviewCutoff(now time.Time) time.Time {
	year, month, day := now.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, now.Location())
}

func ReviewPendingOnly(v *runtime.Reviews) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return
		}
		cutoff := reviewCutoff(time.Now())
		writeJSON(w, map[string]any{"pending": v.Pending(cutoff), "cutoff": cutoff})
	}
}
func ReviewerSelectionView(rt *Runtime, manager *catalog.Manager, ops *OpsState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rt == nil || rt.ReviewerSelection == nil {
			http.Error(w, "reviewer selection unavailable", 503)
			return
		}
		writeJSON(w, reviewerStatus(rt, manager, ops))
	}
}
func ReviewerSelectionSet(rt *Runtime, manager *catalog.Manager, ops *OpsState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || rt == nil || rt.ReviewerSelection == nil {
			http.Error(w, "invalid request", 400)
			return
		}
		var x struct {
			Model    string `json:"model"`
			Provider string `json:"provider"`
		}
		if json.NewDecoder(r.Body).Decode(&x) != nil || x.Model == "" {
			http.Error(w, "model required", 400)
			return
		}
		if !validReviewerModel(manager, ops, x.Model, x.Provider) {
			http.Error(w, "reviewer model unavailable or lacks structured output", 409)
			return
		}
		rt.ReviewerSelection.Set(x.Model, x.Provider, rt.ReviewSession)
		writeJSON(w, map[string]any{"ok": true, "selected": rt.ReviewerSelection.Get()})
	}
}
func ReviewerSelectionClear(rt *Runtime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rt == nil || rt.ReviewerSelection == nil {
			http.Error(w, "unavailable", 503)
			return
		}
		rt.ReviewerSelection.Clear("cleared by user", rt.ReviewSession)
		writeJSON(w, map[string]any{"ok": true})
	}
}
func validReviewerModel(manager *catalog.Manager, ops *OpsState, id, pid string) bool {
	if manager == nil || ops == nil || ops.IsDisabled(pid) {
		return false
	}
	for _, m := range manager.Snapshot().Models {
		if m.ID == id && m.Provider == pid && m.Status == catalog.Active && m.AutoRoutable && m.StructuredOutputKnown && m.StructuredOutput {
			return true
		}
	}
	return false
}
func RunReviewBatch(ctx context.Context, rt *Runtime, manager *catalog.Manager, ops *OpsState, limit int) (map[string]any, error) {
	return RunReviewBatchBefore(ctx, rt, manager, ops, limit, reviewCutoff(time.Now()))
}

func RunReviewBatchBefore(ctx context.Context, rt *Runtime, manager *catalog.Manager, ops *OpsState, limit int, before time.Time) (map[string]any, error) {
	if rt == nil || rt.Reviews == nil || rt.ReviewerSelection == nil {
		return nil, ErrReviewerUnavailable
	}
	sel := rt.ReviewerSelection.Get()
	if sel.Model == "" {
		return nil, ErrReviewerNotSelected
	}
	if ops != nil && ops.IsDisabled(sel.Provider) {
		return nil, ErrReviewerProviderUnavailable
	}
	if !validReviewerModel(manager, ops, sel.Model, sel.Provider) {
		return nil, ErrReviewerUnavailable
	}
	pc, ok := provider.LoadConfigs()[sel.Provider]
	if !ok || pc.BaseURL == "" {
		return nil, ErrReviewerProviderUnavailable
	}
	rv := &runtime.Reviewer{Reviews: rt.Reviews, Session: rt.ReviewSession, BaseURL: pc.BaseURL, APIKey: pc.APIKey, Model: sel.Model, Client: &http.Client{Timeout: 90 * time.Second}}
	if manager != nil {
		for _, m := range manager.Snapshot().Models {
			if m.Status == catalog.Active && m.AutoRoutable && !ops.IsDisabled(m.Provider) {
				rv.AvailableModels = append(rv.AvailableModels, runtime.ReviewModel{ID: m.ID, Provider: m.Provider, Status: string(m.Status), AutoRoutable: true, Capabilities: m.Capabilities, Tools: m.Tools, Vision: m.Vision, StructuredOutput: m.StructuredOutput, ContextWindow: m.ContextWindow})
			}
		}
	}
	return rv.Run(ctx, before, limit)
}

var (
	ErrReviewerUnavailable         = errorString("reviewer unavailable")
	ErrReviewerNotSelected         = errorString("reviewer model not selected")
	ErrReviewerProviderUnavailable = errorString("selected reviewer provider unavailable")
)

type errorString string

func (e errorString) Error() string { return string(e) }
func reviewBefore(r *http.Request, now time.Time) time.Time {
	if r.URL.Query().Get("include_today") == "true" {
		return now
	}
	return reviewCutoff(now)
}

func reviewRunLimit(r *http.Request) int {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func ReviewRun(rt *Runtime, manager *catalog.Manager, ops *OpsState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return
		}
		limit := reviewRunLimit(r)
		before := reviewBefore(r, time.Now())
		ctx, cancel := context.WithTimeout(r.Context(), 100*time.Second)
		defer cancel()
		x, e := RunReviewBatchBefore(ctx, rt, manager, ops, limit, before)
		if e != nil {
			if rt != nil && rt.ReviewerSelection != nil {
				threshold := 3
				if e == ErrReviewerProviderUnavailable {
					threshold = 1
				}
				rt.ReviewerSelection.Failure(e.Error(), threshold, rt.ReviewSession)
			}
			code := 503
			if e == ErrReviewerNotSelected {
				code = 409
			}
			http.Error(w, e.Error(), code)
			return
		}
		if rt != nil && rt.ReviewerSelection != nil {
			rt.ReviewerSelection.Success()
		}
		writeJSON(w, x)
	}
}
func ReviewPrune(rt *Runtime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || rt == nil {
			http.Error(w, "unavailable", 503)
			return
		}
		days := 30
		if x, e := strconv.Atoi(r.URL.Query().Get("days")); e == nil && x > 0 {
			days = x
		}
		cut := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
		a, b := 0, 0
		if rt.Reviews != nil {
			a = rt.Reviews.Prune(cut)
		}
		if rt.Shadow != nil {
			b = rt.Shadow.Prune(cut)
		}
		writeJSON(w, map[string]any{"reviews": a, "shadow": b, "days": days})
	}
}
func ReviewStatsView(v *runtime.Reviews) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats := map[string]int{"pending": 0, "processing": 0, "reviewed": 0, "failed": 0, "skipped": 0}
		if v != nil {
			for _, x := range v.All() {
				stats[string(x.ReviewStatus)]++
			}
		}
		writeJSON(w, map[string]any{"stats": stats})
	}
}
func StartReviewWorker(ctx context.Context, rt *Runtime, manager *catalog.Manager, ops *OpsState, interval time.Duration) {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	go func() {
		run := func() {
			c, cancel := context.WithTimeout(ctx, 100*time.Second)
			defer cancel()
			if _, e := RunReviewBatch(c, rt, manager, ops, 20); e != nil && e != ErrReviewerNotSelected {
			}
		}
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		timer := time.NewTimer(time.Until(next))
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			run()
		}
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	}()
}

var reviewWorkerOnce sync.Once

func StartReviewWorkerOnce(ctx context.Context, rt *Runtime, manager *catalog.Manager, ops *OpsState, interval time.Duration) {
	reviewWorkerOnce.Do(func() { StartReviewWorker(ctx, rt, manager, ops, interval) })
}
