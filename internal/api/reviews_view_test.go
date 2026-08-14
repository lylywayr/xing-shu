package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"xing-shu/internal/runtime"
)

func TestReviewsViewReturnsBoundedSummaryByDefault(t *testing.T) {
	reviews := runtime.NewReviews()
	reviews.Add(runtime.Review{ID: "r1", Model: "m1", Provider: "p1", ReviewStatus: runtime.StatusPending, TaskPackage: []byte(strings.Repeat("task", 10000)), ResponsePackage: []byte(strings.Repeat("response", 10000)), Replayable: true, ResponseCaptured: true})
	w := httptest.NewRecorder()
	ReviewsView(reviews).ServeHTTP(w, httptest.NewRequest("GET", "/api/admin/reviews", nil))
	if w.Code != 200 {
		t.Fatalf("want 200 got %d", w.Code)
	}
	var got struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
		Limit int              `json:"limit"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || got.Limit != 50 || len(got.Items) != 1 {
		t.Fatalf("unexpected page: %+v", got)
	}
	if _, ok := got.Items[0]["task_package"]; ok {
		t.Fatal("summary must not include task package")
	}
	if _, ok := got.Items[0]["response_package"]; ok {
		t.Fatal("summary must not include response package")
	}
	if got.Items[0]["replayable"] != true || got.Items[0]["response_captured"] != true {
		t.Fatal("summary flags missing")
	}
}
