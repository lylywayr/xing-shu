package api

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestReviewBeforeManualIncludesToday(t *testing.T) {
	req := httptest.NewRequest("POST", "/v2/admin/reviews/run?include_today=true", nil)
	now := time.Date(2026, 8, 8, 21, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	if got := reviewBefore(req, now); !got.Equal(now) {
		t.Fatalf("manual run should include today: %v", got)
	}
}
