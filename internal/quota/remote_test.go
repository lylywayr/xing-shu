package quota

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestParseRemoteSnapshotSupportsNewAPIFields(t *testing.T) {
	got, err := ParseRemoteSnapshot("cctq", []byte(`{"data":{"balance":12.5,"used":7.5,"reset_at":"2026-09-01T00:00:00Z"}}`), time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "cctq" || got.State != Available || got.Remaining == nil || got.ResetAt == nil {
		t.Fatalf("missing fields: %+v", got)
	}
	if fmt.Sprint(*got.Remaining) != "12.5" {
		t.Fatalf("remaining=%v", *got.Remaining)
	}
}

func TestRemoteCollectorMarksMissingQuotaUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"data":{"models":[]}}`)) }))
	defer srv.Close()
	manager := NewManager()
	collector := RemoteCollector{Provider: "cctq", URL: srv.URL, Client: srv.Client()}
	if err := collector.Refresh(context.Background(), manager); err == nil {
		t.Fatal("missing quota fields should return an error")
	}
	items := manager.All()
	if len(items) != 1 || items[0].State != Unknown || items[0].ErrorType == "" {
		t.Fatalf("missing unavailable state: %+v", items)
	}
}

func TestParseRemoteSnapshotSupportsUsageAndLimit(t *testing.T) {
	got, err := ParseRemoteSnapshot("newapi", []byte(`{"balance":5,"used":95,"limit":100}`), time.Now().UTC())
	if err != nil || got.Remaining == nil || got.Used == nil || got.Limit == nil {
		t.Fatalf("missing quota facts: %+v err=%v", got, err)
	}
	if *got.Remaining != 5 || *got.Used != 95 || *got.Limit != 100 {
		t.Fatalf("unexpected quota facts: remaining=%v used=%v limit=%v", *got.Remaining, *got.Used, *got.Limit)
	}
}

func TestParseRemoteSnapshotSupportsOfficialNewAPIResponse(t *testing.T) {
	got, err := ParseRemoteSnapshot("newapi", []byte(`{"code":true,"data":{"total_granted":100,"total_used":95,"total_available":5,"expires_at":1798848000}}`), time.Unix(0, 0).UTC())
	if err != nil || got.Remaining == nil || got.Used == nil || got.Limit == nil || got.ResetAt == nil {
		t.Fatalf("unexpected NewAPI facts: %+v err=%v", got, err)
	}
	if *got.Remaining != 5 || *got.Used != 95 || *got.Limit != 100 || got.ResetAt.Unix() != 1798848000 {
		t.Fatalf("unexpected NewAPI values: %+v", got)
	}
}

func TestRemoteCollectorRetriesTransientUpstreamFailure(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			http.Error(w, "temporary", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"balance":9}`))
	}))
	defer srv.Close()
	manager := NewManager()
	collector := RemoteCollector{Provider: "newapi", URL: srv.URL, Client: srv.Client(), Retries: 2, RetryDelay: time.Millisecond}
	if err := collector.Refresh(context.Background(), manager); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 upstream calls, got %d", calls)
	}
}

func TestRemoteCollectorDoesNotRetryUnauthorized(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "denied", http.StatusUnauthorized)
	}))
	defer srv.Close()
	manager := NewManager()
	collector := RemoteCollector{Provider: "newapi", URL: srv.URL, Client: srv.Client(), Retries: 3, RetryDelay: time.Millisecond}
	if err := collector.Refresh(context.Background(), manager); err == nil {
		t.Fatal("unauthorized response should fail")
	}
	if calls != 1 {
		t.Fatalf("unauthorized response was retried %d times", calls)
	}
}

func TestRemoteCollectorRejectsNonHTTPURL(t *testing.T) {
	manager := NewManager()
	collector := RemoteCollector{Provider: "local", URL: "file:///etc/passwd"}
	if err := collector.Refresh(context.Background(), manager); err == nil {
		t.Fatal("non-http quota URL should be rejected")
	}
	if got, ok := manager.Get("local"); !ok || got.State != Unknown {
		t.Fatalf("expected unknown state: %+v %v", got, ok)
	}
}

func TestFileCollectorRejectsNilManager(t *testing.T) {
	path := t.TempDir() + "/quota.json"
	if err := os.WriteFile(path, []byte(`{"balance":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := (FileCollector{Provider: "local", Path: path}).Refresh(nil); err == nil {
		t.Fatal("file collector should reject nil manager")
	}
}
