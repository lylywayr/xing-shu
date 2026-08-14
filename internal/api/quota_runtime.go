package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"xing-shu/internal/provider"
	"xing-shu/internal/quota"
)

type QuotaNotifier interface {
	Send(context.Context, string) error
}

type QuotaRuntime struct {
	Manager                   *quota.Manager
	Remote                    []quota.RemoteCollector
	Files                     []quota.FileCollector
	StatePath                 string
	AlertPath                 string
	AlertStatePath            string
	Notifier                  QuotaNotifier
	LowBalancePercent         float64
	LowBalanceAbsolute        float64
	NotificationRetryInterval time.Duration
	Status                    http.HandlerFunc
	Refresh                   http.HandlerFunc
	alertMu                   sync.Mutex
	refreshMu                 sync.Mutex
	lastRefreshAt             time.Time
	lastRefreshOK             bool
}

type quotaAlertState struct {
	Active       bool      `json:"active"`
	LastNotified time.Time `json:"last_notified,omitempty"`
	LastAlertID  string    `json:"last_alert_id,omitempty"`
}

func NewQuotaRuntime(manager *quota.Manager, remote []quota.RemoteCollector, files []quota.FileCollector) *QuotaRuntime {
	q := &QuotaRuntime{Manager: manager, Remote: remote, Files: files, LowBalancePercent: 10, NotificationRetryInterval: 30 * time.Minute}
	q.Status = q.status
	q.Refresh = q.refresh
	return q
}
func (q *QuotaRuntime) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	items := []quota.Snapshot{}
	if q.Manager != nil {
		items = q.Manager.All()
	}
	available := quota.HasFacts(items)
	status := "unavailable"
	if available {
		status = "available"
	}
	q.refreshMu.Lock()
	lastRefresh := q.lastRefreshAt
	lastRefreshOK := q.lastRefreshOK
	q.refreshMu.Unlock()
	writeJSON(w, map[string]any{"configured": len(q.Remote)+len(q.Files) > 0, "available": available, "status": status, "sources": q.sources(), "items": items, "last_refresh_at": lastRefresh, "last_refresh_ok": lastRefreshOK, "notification_configured": q.Notifier != nil, "message": func() string {
		if !available {
			return "Provider has not supplied quota data"
		}
		return ""
	}()})
}
func (q *QuotaRuntime) refresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if q.Manager == nil {
		http.Error(w, "quota unavailable", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	results := q.RefreshContext(ctx)
	writeJSON(w, map[string]any{"ok": quotaResultsOK(results), "results": results, "items": q.Manager.All()})
}
func (q *QuotaRuntime) RefreshContext(ctx context.Context) map[string]any {
	q.refreshMu.Lock()
	defer q.refreshMu.Unlock()
	results := map[string]any{}
	if q.Manager == nil {
		return results
	}
	for _, collector := range q.Remote {
		results[collector.Provider] = q.collectRemote(ctx, collector)
	}
	for _, collector := range q.Files {
		err := collector.Refresh(q.Manager)
		results[collector.Provider] = map[string]any{"ok": err == nil, "error": safeQuotaError(err)}
	}
	if q.StatePath != "" {
		if err := q.Manager.Save(q.StatePath); err != nil {
			results["state"] = map[string]any{"ok": false, "error": "quota state unavailable"}
		}
	}
	q.evaluateAlerts(ctx)
	q.lastRefreshAt = time.Now().UTC()
	q.lastRefreshOK = true
	for _, item := range results {
		if row, ok := item.(map[string]any); ok && row["ok"] == false {
			q.lastRefreshOK = false
		}
	}
	return results
}
func (q *QuotaRuntime) collectRemote(ctx context.Context, collector quota.RemoteCollector) map[string]any {
	err := collector.Refresh(ctx, q.Manager)
	return map[string]any{"ok": err == nil, "error": safeQuotaError(err)}
}
func (q *QuotaRuntime) sources() []map[string]any {
	out := make([]map[string]any, 0, len(q.Remote)+len(q.Files))
	for _, x := range q.Remote {
		out = append(out, map[string]any{"provider": x.Provider, "type": "remote", "configured": strings.TrimSpace(x.URL) != ""})
	}
	for _, x := range q.Files {
		out = append(out, map[string]any{"provider": x.Provider, "type": "file", "configured": strings.TrimSpace(x.Path) != ""})
	}
	return out
}
func safeQuotaError(err error) string {
	if err == nil {
		return ""
	}
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "timeout") || strings.Contains(text, "deadline") {
		return "quota_timeout"
	}
	if strings.Contains(text, "http 401") || strings.Contains(text, "http 403") {
		return "quota_unauthorized"
	}
	if strings.Contains(text, "http 429") {
		return "quota_rate_limited"
	}
	return "quota_unavailable"
}
func NewQuotaRuntimeFromEnv(manager *quota.Manager) *QuotaRuntime {
	return NewQuotaRuntimeFromEnvWithConfigs(manager, nil)
}
func NewQuotaRuntimeFromEnvWithConfigs(manager *quota.Manager, _ map[string]provider.Config) *QuotaRuntime {
	q := NewQuotaRuntime(manager, nil, nil)
	dataDir := envOr("XING_SHU_DATA_DIR", "/data")
	q.StatePath = filepath.Join(dataDir, "quota-facts-xing-shu.json")
	if manager != nil {
		if err := manager.Load(q.StatePath); err != nil && !os.IsNotExist(err) {
			q.lastRefreshOK = false
		}
	}
	return q
}
func quotaCollector(provider, rawURL, key string) quota.RemoteCollector {
	return quota.RemoteCollector{Provider: provider, URL: rawURL, APIKey: key, Retries: envInt("QUOTA_UPSTREAM_RETRIES", 2), RetryDelay: time.Duration(envInt("QUOTA_RETRY_DELAY_MS", 250)) * time.Millisecond}
}
func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
func envInt(key string, fallback int) int {
	if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil && value >= 0 {
		return value
	}
	return fallback
}
func envFloat(key string, fallback float64) float64 {
	if value, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv(key)), 64); err == nil && value >= 0 {
		return value
	}
	return fallback
}

func (q *QuotaRuntime) evaluateAlerts(ctx context.Context) {
	if q.Manager == nil || q.AlertPath == "" {
		return
	}
	q.alertMu.Lock()
	defer q.alertMu.Unlock()
	states := q.loadQuotaAlertStates()
	for _, item := range q.Manager.All() {
		state := states[item.Provider]
		low := quotaIsLow(item, q.LowBalancePercent, q.LowBalanceAbsolute)
		if low {
			message := quotaAlertMessage(item)
			if !state.Active {
				state.Active = true
				state.LastAlertID = "quota-" + item.Provider + "-low"
				_ = appendQuotaAlert(q.AlertPath, map[string]any{"id": state.LastAlertID, "level": "warning", "type": "quota_low", "provider": item.Provider, "message": message, "at": time.Now().UTC()})
			}
			if q.shouldNotify(state) && q.Notifier != nil {
				if q.Notifier.Send(ctx, message) == nil {
					state.LastNotified = time.Now().UTC()
				}
			}
		} else if state.Active && (item.State == quota.Available || item.State == quota.Exhausted) {
			state.Active = false
			message := fmt.Sprintf("额度恢复：%s 当前状态=%s", item.Provider, item.State)
			_ = appendQuotaAlert(q.AlertPath, map[string]any{"id": state.LastAlertID + "-recovered", "level": "info", "type": "quota_recovered", "provider": item.Provider, "message": message, "at": time.Now().UTC()})
			if q.Notifier != nil {
				if q.Notifier.Send(ctx, message) == nil {
					state.LastNotified = time.Now().UTC()
				}
			}
		}
		states[item.Provider] = state
	}
	q.saveQuotaAlertStates(states)
}
func quotaIsLow(item quota.Snapshot, percent, absolute float64) bool {
	if item.State == quota.Exhausted {
		return true
	}
	if item.State != quota.Available || item.Remaining == nil {
		return false
	}
	if absolute > 0 && *item.Remaining <= absolute {
		return true
	}
	return percent > 0 && item.Limit != nil && *item.Limit > 0 && (*item.Remaining / *item.Limit * 100) <= percent
}
func quotaAlertMessage(item quota.Snapshot) string {
	if item.Remaining == nil {
		return fmt.Sprintf("额度告警：%s 状态=%s", item.Provider, item.State)
	}
	if item.Limit != nil && *item.Limit > 0 {
		return fmt.Sprintf("额度告警：%s 剩余 %.2f / %.2f", item.Provider, *item.Remaining, *item.Limit)
	}
	return fmt.Sprintf("额度告警：%s 剩余 %.2f", item.Provider, *item.Remaining)
}
func (q *QuotaRuntime) shouldNotify(state quotaAlertState) bool {
	if q.Notifier == nil {
		return false
	}
	if state.LastNotified.IsZero() {
		return true
	}
	return time.Since(state.LastNotified) >= q.NotificationRetryInterval
}
func (q *QuotaRuntime) quotaAlertStatePath() string {
	if q.AlertStatePath != "" {
		return q.AlertStatePath
	}
	return filepath.Join(filepath.Dir(q.AlertPath), "quota-alert-state-xing-shu.json")
}
func (q *QuotaRuntime) loadQuotaAlertStates() map[string]quotaAlertState {
	out := map[string]quotaAlertState{}
	body, err := os.ReadFile(q.quotaAlertStatePath())
	if err == nil {
		_ = json.Unmarshal(body, &out)
	}
	return out
}
func (q *QuotaRuntime) saveQuotaAlertStates(states map[string]quotaAlertState) {
	body, err := json.Marshal(states)
	if err != nil {
		return
	}
	path := q.quotaAlertStatePath()
	if os.MkdirAll(filepath.Dir(path), 0750) != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, body, 0600) == nil {
		_ = os.Rename(tmp, path)
	}
}
func appendQuotaAlert(path string, item map[string]any) error {
	body, err := json.Marshal(item)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(body, '\n'))
	return err
}

func (q *QuotaRuntime) StartPeriodic(ctx context.Context, raw string) {
	seconds, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || seconds <= 0 {
		return
	}
	interval := time.Duration(seconds) * time.Second
	go func() {
		q.RefreshContext(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				q.RefreshContext(ctx)
			}
		}
	}()
}

func quotaResultsOK(results map[string]any) bool {
	for _, item := range results {
		row, ok := item.(map[string]any)
		if ok && row["ok"] == false {
			return false
		}
	}
	return true
}
