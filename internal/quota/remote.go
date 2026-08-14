package quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type RemoteCollector struct {
	Provider   string
	URL        string
	APIKey     string
	Client     *http.Client
	Retries    int
	RetryDelay time.Duration
}

func (c RemoteCollector) Refresh(ctx context.Context, manager *Manager) error {
	if manager == nil {
		return errors.New("quota manager unavailable")
	}
	if strings.TrimSpace(c.URL) == "" {
		return c.record(manager, errors.New("quota URL not configured"))
	}
	parsedURL, parseErr := url.Parse(c.URL)
	if parseErr != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return c.record(manager, errors.New("quota URL invalid"))
	}
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	attempts := c.Retries + 1
	if attempts < 1 {
		attempts = 1
	}
	if attempts > 4 {
		attempts = 4
	}
	var last error
	for attempt := 0; attempt < attempts; attempt++ {
		raw, status, err := c.request(ctx, client)
		if err == nil && status >= 200 && status < 300 {
			snapshot, parseErr := ParseRemoteSnapshot(c.Provider, raw, time.Now().UTC())
			if parseErr != nil {
				return c.record(manager, parseErr)
			}
			manager.Set(snapshot)
			return nil
		}
		if err != nil {
			last = err
		} else {
			last = fmt.Errorf("quota HTTP %d", status)
		}
		if !retryQuota(status, err) || attempt == attempts-1 {
			break
		}
		if !waitQuotaRetry(ctx, c.RetryDelay, attempt) {
			return c.record(manager, ctx.Err())
		}
	}
	return c.record(manager, last)
}
func (c RemoteCollector) request(ctx context.Context, client *http.Client) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return nil, 0, err
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	return raw, res.StatusCode, err
}
func retryQuota(status int, err error) bool {
	if err != nil {
		return true
	}
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}
func waitQuotaRetry(ctx context.Context, base time.Duration, attempt int) bool {
	if base <= 0 {
		base = 100 * time.Millisecond
	}
	delay := base * time.Duration(1<<attempt)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
func (c RemoteCollector) record(manager *Manager, err error) error {
	manager.Set(Snapshot{Provider: c.Provider, State: Unknown, CheckedAt: time.Now().UTC(), ErrorType: "quota_unavailable"})
	return err
}
func ParseRemoteSnapshot(provider string, raw []byte, checked time.Time) (Snapshot, error) {
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return Snapshot{}, err
	}
	fields := map[string]any{}
	collectQuotaFields(root, fields)
	remaining, ok := numberField(fields, "remaining", "balance", "credits", "quota", "available", "remain", "remain_quota", "remaining_quota", "quota_remaining")
	used, hasUsed := numberField(fields, "used", "usage", "used_quota", "consumed", "consumption", "total_used")
	limit, hasLimit := numberField(fields, "limit", "total", "quota_limit", "quota_total", "total_granted")
	if available, availableOK := numberField(fields, "total_available"); availableOK {
		remaining, ok = available, true
	}
	if !ok && hasLimit && hasUsed {
		remaining, ok = limit-used, true
	}
	if !ok {
		return Snapshot{Provider: provider, State: Unknown, CheckedAt: checked, ErrorType: "quota_unavailable"}, errors.New("quota balance field unavailable")
	}
	state := Available
	if remaining <= 0 {
		state = Exhausted
	}
	snap := Snapshot{Provider: provider, State: state, CheckedAt: checked, Remaining: floatPointer(remaining)}
	if hasUsed {
		snap.Used = floatPointer(used)
	}
	if hasLimit {
		snap.Limit = floatPointer(limit)
	}
	if parsed, ok := quotaResetTime(fields); ok {
		snap.ResetAt = &parsed
	}
	return snap, nil
}
func collectQuotaFields(value any, fields map[string]any) {
	switch x := value.(type) {
	case map[string]any:
		for key, v := range x {
			fields[key] = v
			collectQuotaFields(v, fields)
		}
	case []any:
		for _, v := range x {
			collectQuotaFields(v, fields)
		}
	}
}
func numberField(fields map[string]any, names ...string) (float64, bool) {
	for _, name := range names {
		if value, ok := fields[name]; ok {
			switch x := value.(type) {
			case float64:
				return x, true
			case json.Number:
				n, err := x.Float64()
				if err == nil {
					return n, true
				}
			case string:
				n, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
				if err == nil {
					return n, true
				}
			}
		}
	}
	return 0, false
}
func stringField(fields map[string]any, names ...string) string {
	for _, name := range names {
		if value, ok := fields[name].(string); ok {
			return value
		}
	}
	return ""
}

type FileCollector struct {
	Provider string
	Path     string
}

func (c FileCollector) Refresh(manager *Manager) error {
	if manager == nil {
		return errors.New("quota manager unavailable")
	}
	raw, err := os.ReadFile(c.Path)
	if err != nil {
		return RemoteCollector{Provider: c.Provider}.record(manager, err)
	}
	snapshot, err := ParseRemoteSnapshot(c.Provider, raw, time.Now().UTC())
	if err != nil {
		return RemoteCollector{Provider: c.Provider}.record(manager, err)
	}
	manager.Set(snapshot)
	return nil
}

func floatPointer(value float64) *float64 { return &value }
func parseQuotaTime(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, errors.New("invalid quota reset time")
}

func quotaResetTime(fields map[string]any) (time.Time, bool) {
	for _, key := range []string{"reset_at", "reset_time", "resetAt", "renew_at", "expired_at", "expires_at"} {
		if raw, ok := fields[key]; ok {
			if text, ok := raw.(string); ok {
				if parsed, err := parseQuotaTime(text); err == nil {
					return parsed, true
				}
			}
			if number, ok := numberField(fields, key); ok && number > 0 {
				if number > 1e12 {
					number /= 1000
				}
				return time.Unix(int64(number), 0).UTC(), true
			}
		}
	}
	return time.Time{}, false
}
