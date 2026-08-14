package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"xing-shu/internal/quota"
)

func TestQuotaRuntimeStatusReturnsUnavailableWithoutSources(t *testing.T) {
	q := NewQuotaRuntime(quota.NewManager(), nil, nil)
	w := httptest.NewRecorder()
	q.Status.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/admin/quota/status", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "unavailable") {
		t.Fatalf("unexpected quota status: %d %s", w.Code, w.Body.String())
	}
}

func TestQuotaRuntimeRefreshReportsConfiguredSources(t *testing.T) {
	q := NewQuotaRuntime(quota.NewManager(), []quota.RemoteCollector{{Provider: "cctq", URL: ""}}, nil)
	w := httptest.NewRecorder()
	q.Refresh.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/admin/quota/refresh", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "cctq") {
		t.Fatalf("unexpected quota refresh: %d %s", w.Code, w.Body.String())
	}
}

type quotaTestNotifier struct {
	messages []string
}

func (n *quotaTestNotifier) Send(_ context.Context, message string) error {
	n.messages = append(n.messages, message)
	return nil
}

func TestQuotaRuntimeRefreshPersistsAndDeduplicatesLowBalanceAlert(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"total_granted":100,"total_used":95,"total_available":5}}`))
	}))
	defer server.Close()
	dir := t.TempDir()
	notifier := &quotaTestNotifier{}
	q := NewQuotaRuntime(quota.NewManager(), []quota.RemoteCollector{{Provider: "newapi", URL: server.URL, Client: server.Client()}}, nil)
	q.StatePath, q.AlertPath, q.AlertStatePath, q.Notifier = dir+"/quota.json", dir+"/alerts.jsonl", dir+"/alert-state.json", notifier
	q.NotificationRetryInterval = time.Hour
	q.RefreshContext(context.Background())
	q.RefreshContext(context.Background())
	if len(notifier.messages) != 1 {
		t.Fatalf("expected one deduplicated notification, got %d", len(notifier.messages))
	}
	alerts := ReadJSONL(q.AlertPath)
	if len(alerts) != 1 || alerts[0]["provider"] != "newapi" {
		t.Fatalf("unexpected quota alerts: %+v", alerts)
	}
	loaded := quota.NewManager()
	if err := loaded.Load(q.StatePath); err != nil || len(loaded.All()) != 1 {
		t.Fatalf("quota state was not persisted: err=%v items=%+v", err, loaded.All())
	}
}

func TestQuotaRuntimeRefreshReportsSafeErrorCode(t *testing.T) {
	q := NewQuotaRuntime(quota.NewManager(), []quota.RemoteCollector{{Provider: "cctq", URL: "http://[secret.invalid"}}, nil)
	q.StatePath = t.TempDir() + "/quota.json"
	result := q.RefreshContext(context.Background())
	item, ok := result["cctq"].(map[string]any)
	if !ok || item["ok"] == true || item["error"] == "" || strings.Contains(item["error"].(string), "secret") {
		t.Fatalf("unsafe quota result: %+v", result)
	}
}
