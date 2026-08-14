package api

import (
	"testing"
	"time"
)

func TestReviewCutoffUsesLocalMidnight(t *testing.T) {
	zone := time.FixedZone("CST", 8*60*60)
	now := time.Date(2026, 8, 8, 21, 0, 0, 0, zone)
	got := reviewCutoff(now)
	want := time.Date(2026, 8, 8, 0, 0, 0, 0, zone)
	if !got.Equal(want) {
		t.Fatalf("want %v got %v", want, got)
	}
	if !time.Date(2026, 8, 8, 10, 31, 0, 0, zone).Before(got) {
		return
	}
}
