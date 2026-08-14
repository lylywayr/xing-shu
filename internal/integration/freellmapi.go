package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"xing-shu/internal/provider"
	"xing-shu/internal/quota"
)

const FreeLLMAPIID = "freellmapi"
const FreeLLMAPIInstanceID = "freellmapi-local"

// State is persisted without credentials. It records operator consent and
// last-known facts about one external FreeLLMAPI instance.
type State struct {
	InstanceID        string    `json:"instance_id"`
	Adapter           string    `json:"adapter"`
	Endpoint          string    `json:"endpoint"`
	SecretRef         string    `json:"secret_ref"`
	Authorized        bool      `json:"authorized"`
	RouteEnabled      bool      `json:"route_enabled"`
	Recognized        bool      `json:"recognized"`
	Connected         bool      `json:"connected"`
	CatalogFresh      bool      `json:"catalog_fresh"`
	Usable            bool      `json:"usable"`
	Status            string    `json:"status"`
	Release           string    `json:"release,omitempty"`
	ModelCount        int       `json:"model_count"`
	AvailableModels   int       `json:"available_models"`
	ReadyUpstreams    int       `json:"ready_upstreams"`
	SummarySupported  bool      `json:"summary_supported"`
	LastProbe         time.Time `json:"last_probe,omitempty"`
	LastSync          time.Time `json:"last_sync,omitempty"`
	LastQuota         time.Time `json:"last_quota,omitempty"`
	LastError         string    `json:"last_error,omitempty"`
	Generation        uint64    `json:"generation"`
	ConfigFingerprint string    `json:"config_fingerprint,omitempty"`
	RevokedAt         time.Time `json:"revoked_at,omitempty"`
}

type persisted struct {
	FreeLLMAPI          State             `json:"freellmapi"`
	LocalQuotaDBPath    string            `json:"local_quota_db_path,omitempty"`
	LocalQuota          *LocalQuotaReport `json:"local_quota,omitempty"`
	LocalQuotaCheckedAt time.Time         `json:"local_quota_checked_at,omitempty"`
	LocalQuotaError     string            `json:"local_quota_error,omitempty"`
}

type QuotaPool struct {
	ID            string   `json:"id"`
	Platform      string   `json:"platform,omitempty"`
	KeyID         *int64   `json:"key_id,omitempty"`
	ModelID       string   `json:"model_id,omitempty"`
	Metric        string   `json:"metric"`
	Unit          string   `json:"unit,omitempty"`
	Limit         *float64 `json:"limit"`
	Used          *float64 `json:"used"`
	Remaining     *float64 `json:"remaining"`
	ResetAt       string   `json:"reset_at,omitempty"`
	ResetStrategy string   `json:"reset_strategy,omitempty"`
	Source        string   `json:"source"`
	Confidence    float64  `json:"confidence"`
	Notes         string   `json:"notes,omitempty"`
	ObservedAt    string   `json:"observed_at,omitempty"`
	Partial       bool     `json:"partial,omitempty"`
}

type Summary struct {
	SchemaVersion int       `json:"schema_version"`
	Service       string    `json:"service"`
	Release       string    `json:"release,omitempty"`
	CheckedAt     time.Time `json:"checked_at"`
	Models        struct {
		Total     int `json:"total"`
		Available int `json:"available"`
	} `json:"models"`
	ReadyUpstreams int             `json:"ready_upstreams"`
	Capabilities   map[string]bool `json:"capabilities"`
	QuotaPools     []QuotaPool     `json:"quota_pools"`
	Partial        bool            `json:"partial"`
}

type Report struct {
	State         State
	Models        []provider.RawModel
	Providers     []map[string]any
	Summary       *Summary
	SummaryStatus int
	SummaryError  string
}

type Manager struct {
	mu       sync.RWMutex
	path     string
	key      string
	client   *http.Client
	instance State

	localDBPath         string
	localQuota          *LocalQuotaReport
	localQuotaError     string
	localQuotaCheckedAt time.Time
}

func validateEndpoint(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Hostname() == "" {
		return false
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	port := parsed.Port()
	if port != "" && port != "80" && port != "443" && port != "3001" {
		return false
	}
	return true
}

func rootEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !validateEndpoint(raw) {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ""
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	if strings.HasSuffix(parsed.Path, "/v1") {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/v1")
	}
	parsed.RawQuery, parsed.Fragment = "", ""
	return strings.TrimRight(parsed.String(), "/")
}

func New(path, rawEndpoint, key string, defaultAuthorized bool) (*Manager, error) {
	endpoint := rootEndpoint(rawEndpoint)
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	if endpoint == "" || strings.TrimSpace(key) == "" {
		return &Manager{path: path, key: key, client: client, instance: State{InstanceID: FreeLLMAPIInstanceID, Adapter: "freellmapi", Endpoint: endpoint, SecretRef: "FREELLMAPI_KEY", Status: "unconfigured"}}, nil
	}
	m := &Manager{path: path, key: key, client: client}
	m.instance = State{InstanceID: FreeLLMAPIInstanceID, Adapter: "freellmapi", Endpoint: endpoint, SecretRef: "FREELLMAPI_KEY", Status: "unauthorized"}
	m.instance.ConfigFingerprint = fingerprint(endpoint, key)
	saved := false
	body, err := os.ReadFile(path)
	if err == nil {
		var savedState persisted
		if json.Unmarshal(body, &savedState) == nil && savedState.FreeLLMAPI.InstanceID != "" {
			saved = true
			m.instance = savedState.FreeLLMAPI
			m.localDBPath = savedState.LocalQuotaDBPath
			m.localQuota = savedState.LocalQuota
			m.localQuotaCheckedAt = savedState.LocalQuotaCheckedAt
			m.localQuotaError = savedState.LocalQuotaError
			if m.instance.Endpoint != endpoint || m.instance.ConfigFingerprint != fingerprint(endpoint, key) {
				m.instance.Endpoint = endpoint
				m.instance.ConfigFingerprint = fingerprint(endpoint, key)
				m.instance.Authorized = false
				m.instance.RouteEnabled = false
				m.instance.Usable = false
				m.instance.Status = "config_changed"
				m.localQuota = nil
				m.localQuotaCheckedAt = time.Time{}
				m.localQuotaError = "config_changed"
			}
		}
	}
	if m.instance.InstanceID == "" {
		m.instance.InstanceID = FreeLLMAPIInstanceID
	}
	if !saved && defaultAuthorized {
		m.instance.Authorized = true
		m.instance.RouteEnabled = true
		m.instance.Status = "authorized"
	}
	_ = m.persistLocked()
	return m, nil
}

func fingerprint(endpoint, key string) string {
	h := sha256.Sum256([]byte(endpoint + "\x00" + key))
	return hex.EncodeToString(h[:])
}

func (m *Manager) LocalQuotaState() (report *LocalQuotaReport, checkedAt time.Time, errText string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneLocalQuota(m.localQuota), m.localQuotaCheckedAt, m.localQuotaError
}

func cloneLocalQuota(report *LocalQuotaReport) *LocalQuotaReport {
	if report == nil {
		return nil
	}
	copy := *report
	copy.Models = append([]LocalQuotaModel(nil), report.Models...)
	copy.Pools = append([]QuotaPool(nil), report.Pools...)
	copy.Warnings = append([]string(nil), report.Warnings...)
	return &copy
}

func (m *Manager) SetLocalQuotaPath(path string) {
	clean := filepath.Clean(strings.TrimSpace(path))
	m.mu.Lock()
	if m.localDBPath != clean {
		m.localQuota = nil
		m.localQuotaError = ""
		m.localQuotaCheckedAt = time.Time{}
	}
	m.localDBPath = clean
	_ = m.persistLocked()
	m.mu.Unlock()
}

func (m *Manager) LocalQuotaPath() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.localDBPath
}

func (m *Manager) RefreshLocalQuota(ctx context.Context) (*LocalQuotaReport, error) {
	path := m.LocalQuotaPath()
	report, err := m.ReadLocalQuota(ctx, LocalQuotaConfig{DBPath: path})
	m.mu.Lock()
	m.localQuotaCheckedAt = time.Now().UTC()
	m.instance.LastQuota = m.localQuotaCheckedAt
	if !m.instance.Authorized {
		// Revocation may race a slow read. Never publish or persist data after
		// consent has been withdrawn.
		m.localQuota = nil
		m.localQuotaError = "not_authorized"
		_ = m.persistLocked()
		m.mu.Unlock()
		return nil, errors.New("freellmapi integration not authorized")
	}
	if err != nil {
		// Keep the last-known-good local snapshot. The error is exposed separately
		// so the UI can mark it stale rather than losing usable historical facts.
		m.localQuotaError = safeLocalQuotaError(err)
	} else {
		m.localQuota = report
		m.localQuotaError = ""
	}
	_ = m.persistLocked()
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return report, nil
}

func safeLocalQuotaError(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if strings.Contains(text, "not authorized") {
		return "not_authorized"
	}
	if strings.Contains(text, "not configured") {
		return "local_database_not_configured"
	}
	if strings.Contains(text, "read-only") {
		return "local_database_not_read_only"
	}
	return "local_database_unavailable"
}

func (m *Manager) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.instance
}
func (m *Manager) Authorized() bool { return m.State().Authorized }
func (m *Manager) RouteEnabled() bool {
	x := m.State()
	return x.Authorized && x.RouteEnabled && x.Connected && x.CatalogFresh && x.Usable
}
func (m *Manager) Configured() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.instance.Endpoint != "" && strings.TrimSpace(m.key) != ""
}

func (m *Manager) Config() provider.Config {
	x := m.State()
	base := x.Endpoint
	if base != "" {
		base += "/v1"
	}
	return provider.Config{ID: FreeLLMAPIID, BaseURL: base, APIKey: m.key, Kind: "external_local"}
}
func (m *Manager) persistLocked() error {
	if m.path == "" {
		return nil
	}
	body, err := json.MarshalIndent(persisted{FreeLLMAPI: m.instance, LocalQuotaDBPath: m.localDBPath, LocalQuota: m.localQuota, LocalQuotaCheckedAt: m.localQuotaCheckedAt, LocalQuotaError: m.localQuotaError}, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(m.path), 0700); err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err = os.WriteFile(tmp, body, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, m.path)
}

func (m *Manager) setStatus(fn func(*State)) {
	m.mu.Lock()
	fn(&m.instance)
	_ = m.persistLocked()
	m.mu.Unlock()
}

func (m *Manager) Authorize(route bool) State {
	m.setStatus(func(x *State) {
		x.Authorized = true
		x.RouteEnabled = route
		x.RevokedAt = time.Time{}
		x.Status = "authorized"
		x.Usable = false
		x.Generation++
	})
	// Authorization is the explicit consent boundary for local read-only data.
	// The actual read is triggered by the caller after the consent state commits.
	return m.State()
}
func (m *Manager) Revoke() State {
	m.setStatus(func(x *State) {
		x.Authorized = false
		x.RouteEnabled = false
		x.Usable = false
		x.Status = "revoked"
		x.RevokedAt = time.Now().UTC()
		x.Generation++
	})
	m.mu.Lock()
	m.localQuota = nil
	m.localQuotaError = "not_authorized"
	m.localQuotaCheckedAt = time.Now().UTC()
	_ = m.persistLocked()
	m.mu.Unlock()
	return m.State()
}
func (m *Manager) SetRoute(enabled bool) State {
	m.setStatus(func(x *State) {
		x.RouteEnabled = enabled
		if !enabled {
			x.Usable = false
		} else if x.Authorized && x.Connected && x.CatalogFresh {
			x.Usable = true
		}
		x.Generation++
	})
	return m.State()
}

func (m *Manager) request(ctx context.Context, path string, auth bool) (int, http.Header, []byte, error) {
	if m.State().Endpoint == "" || m.key == "" {
		return 0, nil, nil, errors.New("freellmapi endpoint unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.State().Endpoint+path, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	if auth {
		req.Header.Set("Authorization", "Bearer "+m.key)
	}
	res, err := m.client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer res.Body.Close()
	responseLimit := int64(4 << 20)
	body, err := io.ReadAll(io.LimitReader(res.Body, responseLimit+1))
	if err == nil && int64(len(body)) > responseLimit {
		return res.StatusCode, res.Header, nil, errors.New("freellmapi response too large")
	}
	return res.StatusCode, res.Header, body, err
}

func (m *Manager) AddModels(raw []provider.RawModel) []provider.RawModel {
	out := make([]provider.RawModel, 0, len(raw))
	for _, model := range raw {
		id := strings.TrimSpace(model.ID)
		if id == "" || len(id) > 300 || strings.ContainsAny(id, "\r\n\x00") {
			continue
		}
		lower := strings.ToLower(id)
		if lower == "auto" || lower == "fusion" || strings.HasPrefix(lower, "auto:") {
			continue
		}
		model.ID = id
		if model.Context < 0 || model.Context > 10_000_000 {
			model.Context = 0
		}
		out = append(out, model)
	}
	return out
}

func decodeModels(raw []byte) ([]provider.RawModel, error) {
	var envelope struct {
		Data []provider.RawModel `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Data) == 0 {
		return nil, errors.New("freellmapi returned an empty catalog")
	}
	for _, model := range envelope.Data {
		if strings.TrimSpace(model.ID) == "" || len(model.ID) > 300 {
			return nil, errors.New("freellmapi returned an invalid model id")
		}
	}
	return envelope.Data, nil
}

func decodeProviders(raw []byte) ([]map[string]any, error) {
	var envelope struct {
		Providers []map[string]any `json:"providers"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	if envelope.Providers == nil {
		envelope.Providers = []map[string]any{}
	}
	if len(envelope.Providers) > 1000 {
		return nil, errors.New("freellmapi returned too many providers")
	}
	return envelope.Providers, nil
}

func decodeSummary(raw []byte) (*Summary, error) {
	var summary Summary
	if err := json.Unmarshal(raw, &summary); err != nil {
		return nil, err
	}
	if summary.Service != "freellmapi" && summary.Service != "" {
		return nil, errors.New("unexpected summary service")
	}
	if len(summary.QuotaPools) > 1000 {
		return nil, errors.New("too many quota pools")
	}
	for _, pool := range summary.QuotaPools {
		if pool.Metric == "" || pool.ID == "" || len(pool.ID) > 200 {
			return nil, errors.New("invalid quota pool")
		}
		for _, value := range []*float64{pool.Limit, pool.Used, pool.Remaining} {
			if value != nil && (*value < 0 || *value > 1e18) {
				return nil, errors.New("invalid quota value")
			}
		}
	}
	return &summary, nil
}

func (m *Manager) Probe(ctx context.Context) (Report, error) {
	state := m.State()
	if state.Endpoint == "" || m.key == "" {
		return Report{State: state}, errors.New("freellmapi is not configured")
	}
	liveStatus, _, _, liveErr := m.request(ctx, "/livez", false)
	if liveErr != nil || liveStatus < 200 || liveStatus >= 300 {
		m.setStatus(func(x *State) {
			x.Connected = false
			x.Usable = false
			x.Status = "disconnected"
			x.LastError = safeError(liveErr, liveStatus)
		})
		return Report{State: m.State()}, fmt.Errorf("freellmapi live check failed")
	}
	readyStatus, _, readyRaw, readyErr := m.request(ctx, "/readyz", false)
	if readyErr != nil || readyStatus < 200 || readyStatus >= 300 {
		m.setStatus(func(x *State) {
			x.Connected = true
			x.Recognized = true
			x.Usable = false
			x.Status = "degraded"
			x.LastError = safeError(readyErr, readyStatus)
		})
		return Report{State: m.State()}, fmt.Errorf("freellmapi is not ready")
	}
	modelsStatus, _, modelsRaw, modelsErr := m.request(ctx, "/v1/models", true)
	if modelsErr != nil || modelsStatus < 200 || modelsStatus >= 300 {
		m.setStatus(func(x *State) {
			x.Connected = true
			x.Recognized = false
			x.Usable = false
			x.Status = "unauthorized"
			x.LastError = safeError(modelsErr, modelsStatus)
		})
		return Report{State: m.State()}, fmt.Errorf("freellmapi model access failed")
	}
	models, err := decodeModels(modelsRaw)
	if err != nil {
		m.setStatus(func(x *State) {
			x.Connected = true
			x.Recognized = false
			x.Usable = false
			x.Status = "invalid"
			x.LastError = err.Error()
		})
		return Report{State: m.State()}, err
	}
	models = m.AddModels(models)
	if len(models) == 0 {
		m.setStatus(func(x *State) {
			x.CatalogFresh = false
			x.Usable = false
			x.Status = "invalid"
			x.LastError = "no_routable_models"
		})
		return Report{State: m.State()}, errors.New("freellmapi returned no routable models")
	}
	providersStatus, _, providersRaw, providersErr := m.request(ctx, "/v1/providers", true)
	providers := []map[string]any{}
	if providersErr == nil && providersStatus >= 200 && providersStatus < 300 {
		providers, _ = decodeProviders(providersRaw)
	}
	summaryStatus, _, summaryRaw, summaryErr := m.request(ctx, "/v1/integrations/starcore/summary", true)
	var summary *Summary
	if summaryErr == nil && summaryStatus >= 200 && summaryStatus < 300 {
		summary, summaryErr = decodeSummary(summaryRaw)
		if summaryErr == nil {
			summary.Release = strings.TrimSpace(summary.Release)
			summary.CheckedAt = time.Now().UTC()
		}
	}
	report := Report{State: m.State(), Models: models, Providers: providers, Summary: summary, SummaryStatus: summaryStatus}
	if summaryErr != nil && summaryStatus != http.StatusNotFound {
		report.SummaryError = "summary unavailable"
	}
	m.setStatus(func(x *State) {
		x.Recognized = true
		x.Connected = true
		x.CatalogFresh = true
		x.Usable = x.Authorized && x.RouteEnabled
		x.Status = "connected"
		x.ModelCount = len(models)
		x.ReadyUpstreams = readyUpstreams(readyRaw, providers)
		x.SummarySupported = summary != nil
		x.Release = ""
		x.LastProbe = time.Now().UTC()
		x.LastError = ""
	})
	report.State = m.State()
	return report, nil
}

func (m *Manager) MarkSynced(count int) State {
	m.setStatus(func(x *State) {
		x.ModelCount = count
		x.CatalogFresh = true
		x.LastSync = time.Now().UTC()
		x.LastError = ""
		if x.Connected {
			x.Usable = x.Authorized && x.RouteEnabled
		}
	})
	return m.State()
}

func (m *Manager) ApplySummary(summary *Summary, manager *quota.Manager) {
	if summary == nil || manager == nil {
		return
	}
	for _, pool := range summary.QuotaPools {
		checked := summary.CheckedAt
		if checked.IsZero() {
			checked = time.Now().UTC()
		}
		state := quota.Unknown
		if pool.Remaining != nil {
			state = quota.Available
			if *pool.Remaining <= 0 {
				state = quota.Exhausted
			}
		}
		freshUntil := checked.Add(5 * time.Minute)
		manager.Set(quota.Snapshot{Provider: FreeLLMAPIID, Pool: pool.ID, Metric: pool.Metric, Unit: pool.Unit, State: state, Remaining: pool.Remaining, Used: pool.Used, Limit: pool.Limit, CheckedAt: checked, FreshUntil: &freshUntil, Source: pool.Source, Confidence: pool.Confidence, ResetAt: parseTime(pool.ResetAt), ErrorType: func() string {
			if pool.Partial {
				return "quota_partial"
			}
			return ""
		}()})
	}
	m.setStatus(func(x *State) { x.LastQuota = time.Now().UTC() })
}

func parseTime(value string) *time.Time {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &parsed
}
func readyUpstreams(raw []byte, providers []map[string]any) int {
	var x struct {
		ReadyUpstreams int `json:"ready_upstreams"`
	}
	if json.Unmarshal(raw, &x) == nil && x.ReadyUpstreams >= 0 {
		return x.ReadyUpstreams
	}
	return len(providers)
}
func safeError(err error, status int) string {
	if err != nil {
		return "connection_error"
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return "unauthorized"
	}
	if status == 0 {
		return "unavailable"
	}
	return fmt.Sprintf("http_%d", status)
}
