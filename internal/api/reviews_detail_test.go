package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"xing-shu/internal/runtime"
)

func seededReviews() *runtime.Reviews {
	reviews := runtime.NewReviews()
	reviews.Add(runtime.Review{ID: "r1", RecordedAt: time.Now().Add(-time.Hour), Model: "m1", Provider: "p1", BatchID: "batch-1", ReviewStatus: runtime.StatusFailed, ReviewError: "provider timeout", Replayable: true, TaskPackage: []byte(`{"messages":[{"content":"task"}]}`), ResponsePackage: []byte(`{"choices":[]}`), ResponseCaptured: true})
	reviews.Add(runtime.Review{ID: "r2", RecordedAt: time.Now().Add(-30 * time.Minute), Model: "m2", Provider: "p1", BatchID: "batch-1", ReviewStatus: runtime.StatusReviewed, Replayable: false})
	return reviews
}

func TestReviewDetailHidesRawPackagesByDefaultAndCapsRawOutput(t *testing.T) {
	views := seededReviews()
	handler := ReviewDetailView(views, 8)
	for _, raw := range []string{"", "&raw=true"} {
		req := httptest.NewRequest(http.MethodGet, "/api/admin/reviews/detail?id=r1"+raw, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var body map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if body["task_package"] != nil && raw == "" {
			t.Fatal("raw task package leaked by default")
		}
		if raw != "" && len(body["task_package"].(string)) > 8 {
			t.Fatalf("raw package exceeded cap: %q", body["task_package"])
		}
	}
}

func TestReviewDetailRejectsMissingReviewAndInvalidRawFlag(t *testing.T) {
	handler := ReviewDetailView(seededReviews(), 64)
	for _, path := range []string{"/api/admin/reviews/detail?id=missing", "/api/admin/reviews/detail?id=r1&raw=maybe"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
			t.Fatalf("unexpected status %d for %s", w.Code, path)
		}
	}
}

func TestReviewBatchesAggregatesStatusesAndErrors(t *testing.T) {
	handler := ReviewBatchesView(seededReviews())
	req := httptest.NewRequest(http.MethodGet, "/api/admin/reviews/batches", strings.NewReader(""))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Items) != 1 || body.Items[0]["batch_id"] != "batch-1" || body.Items[0]["total"] != float64(2) || body.Items[0]["failed"] != float64(1) {
		t.Fatalf("unexpected batches: %+v", body.Items)
	}
}
