package api

import (
	"net/http/httptest"
	"testing"
)

func TestReviewRunLimitDefaultsToTwenty(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/admin/reviews/run", nil)
	if got := reviewRunLimit(req); got != 20 {
		t.Fatalf("want default 20 got %d", got)
	}
	req = httptest.NewRequest("POST", "/api/admin/reviews/run?limit=5", nil)
	if got := reviewRunLimit(req); got != 5 {
		t.Fatalf("want explicit 5 got %d", got)
	}
}
