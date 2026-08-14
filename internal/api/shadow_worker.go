package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
	"xing-shu/internal/catalog"
	"xing-shu/internal/runtime"
)

func StartShadowWorker(ctx context.Context, rt *Runtime, manager *catalog.Manager, ops *OpsState, interval time.Duration) {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	go func() {
		run := func() {
			defer func() { _ = recover() }()
			BuildKnowledge(rt)
			if rt == nil || rt.Knowledge == nil || rt.Reviews == nil || rt.Shadow == nil {
				return
			}
			sem := make(chan struct{}, 2)
			var wg sync.WaitGroup
			for _, k := range rt.Knowledge.All() {
				if k.Status != runtime.KnowledgeTrusted || k.RecommendedModel == "" {
					continue
				}
				for _, rev := range rt.Reviews.All() {
					if rev.ReviewStatus != runtime.StatusReviewed || !rev.Replayable || len(rev.TaskPackage) == 0 || !contains(rev.ID, k.SourceReviewIDs) {
						continue
					}
					id := k.ID + "|" + rev.ID + "|" + k.RecommendedModel
					if rt.Shadow.Has(id) {
						continue
					}
					select {
					case sem <- struct{}{}:
					case <-ctx.Done():
						return
					}
					wg.Add(1)
					go func(k runtime.Knowledge, rev runtime.Review) {
						defer wg.Done()
						defer func() { <-sem; _ = recover() }()
						c, cancel := context.WithTimeout(ctx, 130*time.Second)
						defer cancel()
						req := httptest.NewRequestWithContext(c, "POST", "/api/admin/shadow/run", strings.NewReader(`{"knowledge_id":"`+escape(k.ID)+`","review_id":"`+escape(rev.ID)+`","candidate_model":"`+escape(k.RecommendedModel)+`"}`))
						rr := httptest.NewRecorder()
						ShadowRun(rt, manager, ops).ServeHTTP(rr, req)
					}(k, rev)
				}
			}
			wg.Wait()
		}
		timer := time.NewTimer(time.Until(nextAt(4)))
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
func nextAt(hour int) time.Time {
	n := time.Now()
	x := time.Date(n.Year(), n.Month(), n.Day(), hour, 0, 0, 0, n.Location())
	if !x.After(n) {
		x = x.Add(24 * time.Hour)
	}
	return x
}
func contains(id string, xs []string) bool {
	for _, x := range xs {
		if x == id {
			return true
		}
	}
	return false
}
func escape(s string) string { return strings.NewReplacer(`\\`, `\\\\`, `"`, `\\"`).Replace(s) }

var _ catalog.Model
