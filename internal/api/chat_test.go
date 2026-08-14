package api

import (
	"net/http/httptest"
	"strings"
	"testing"
	"xing-shu/internal/catalog"
	"xing-shu/internal/routing"
	"xing-shu/internal/runtime"
)

func TestChatUnknown503(t *testing.T) {
	c := &Chat{Router: &routing.Service{Models: []catalog.Model{}}}
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"unknown","messages":[]}`))
	w := httptest.NewRecorder()
	c.ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatalf("want 503 got %d", w.Code)
	}
}
func TestSessionID(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	r.Header.Set("X-Conversation-ID", "abc")
	if sessionID(r) != "abc" {
		t.Fatal("conversation id not selected")
	}
}
func TestLearningRecord(t *testing.T) {
	l := runtime.NewLearning()
	l.Record("m", true, 0)
	if l.Snapshot()["m"].Success != 1 {
		t.Fatal("learning not recorded")
	}
}
