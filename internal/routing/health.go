package routing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FailureKind string

const (
	FailureNetwork     FailureKind = "network"
	FailureTimeout     FailureKind = "timeout"
	FailureRateLimited FailureKind = "rate_limited"
	FailureServer      FailureKind = "upstream_server"
	FailureAuth        FailureKind = "authentication"
	FailureProtocol    FailureKind = "protocol"
)

type HealthOptions struct{ NetworkCooldown, RateLimitCooldown, ServerCooldown, AuthCooldown, ProtocolCooldown time.Duration }
type RouteHealth struct {
	Provider            string      `json:"provider"`
	Model               string      `json:"model"`
	ConsecutiveFailures int         `json:"consecutive_failures"`
	LastFailure         FailureKind `json:"last_failure,omitempty"`
	CooldownUntil       time.Time   `json:"cooldown_until,omitempty"`
	LastSuccessAt       *time.Time  `json:"last_success_at,omitempty"`
	NeedsProbe          bool        `json:"needs_probe"`
}
type Health struct {
	mu      sync.RWMutex
	options HealthOptions
	items   map[string]RouteHealth
	path    string
}

func NewHealth(o HealthOptions) *Health {
	if o.NetworkCooldown == 0 {
		o.NetworkCooldown = 15 * time.Second
	}
	if o.RateLimitCooldown == 0 {
		o.RateLimitCooldown = time.Minute
	}
	if o.ServerCooldown == 0 {
		o.ServerCooldown = 30 * time.Second
	}
	if o.AuthCooldown == 0 {
		o.AuthCooldown = 30 * time.Minute
	}
	if o.ProtocolCooldown == 0 {
		o.ProtocolCooldown = 5 * time.Minute
	}
	return &Health{options: o, items: map[string]RouteHealth{}}
}
func OpenHealth(path string, o HealthOptions) *Health {
	h := NewHealth(o)
	h.path = path
	if b, err := os.ReadFile(path); err == nil {
		var items map[string]RouteHealth
		if json.Unmarshal(b, &items) == nil && items != nil {
			h.items = items
		}
	}
	return h
}
func (h *Health) persistLocked() {
	if h.path == "" {
		return
	}
	b, err := json.Marshal(h.items)
	if err != nil || os.MkdirAll(filepath.Dir(h.path), 0700) != nil {
		return
	}
	tmp := h.path + ".tmp"
	if os.WriteFile(tmp, b, 0600) == nil {
		_ = os.Rename(tmp, h.path)
	}
}
func healthKey(provider, model string) string { return provider + "/" + model }
func (h *Health) Fail(provider, model string, kind FailureKind, now time.Time) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	key := healthKey(provider, model)
	x := h.items[key]
	x.Provider = provider
	x.Model = model
	x.ConsecutiveFailures++
	x.LastFailure = kind
	x.NeedsProbe = true
	duration := h.options.NetworkCooldown
	switch kind {
	case FailureRateLimited:
		duration = h.options.RateLimitCooldown
	case FailureServer:
		duration = h.options.ServerCooldown
	case FailureAuth:
		duration = h.options.AuthCooldown
	case FailureProtocol:
		duration = h.options.ProtocolCooldown
	case FailureTimeout:
		duration = h.options.NetworkCooldown
	}
	mult := x.ConsecutiveFailures
	if mult > 4 {
		mult = 4
	}
	x.CooldownUntil = now.Add(duration * time.Duration(mult))
	h.items[key] = x
	h.persistLocked()
}
func (h *Health) FailUntil(provider, model string, kind FailureKind, now, until time.Time) {
	h.Fail(provider, model, kind, now)
	if h == nil || !until.After(now) {
		return
	}
	h.mu.Lock()
	key := healthKey(provider, model)
	x := h.items[key]
	if until.After(x.CooldownUntil) {
		x.CooldownUntil = until
		h.items[key] = x
		h.persistLocked()
	}
	h.mu.Unlock()
}
func (h *Health) ProbeFailed(provider, model string, now time.Time) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	key := healthKey(provider, model)
	x := h.items[key]
	x.Provider = provider
	x.Model = model
	x.NeedsProbe = true
	x.ConsecutiveFailures++
	if x.ConsecutiveFailures > 8 {
		x.ConsecutiveFailures = 8
	}
	x.CooldownUntil = now.Add(time.Duration(x.ConsecutiveFailures) * time.Minute)
	h.items[key] = x
	h.persistLocked()
}
func (h *Health) Success(provider, model string, now time.Time) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	x := h.items[healthKey(provider, model)]
	x.Provider = provider
	x.Model = model
	x.ConsecutiveFailures = 0
	x.LastFailure = ""
	x.CooldownUntil = time.Time{}
	x.NeedsProbe = false
	at := now.UTC()
	x.LastSuccessAt = &at
	h.items[healthKey(provider, model)] = x
	h.persistLocked()
}
func (h *Health) Available(provider, model string, now time.Time) bool {
	if h == nil {
		return true
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, key := range []string{healthKey(provider, "*"), healthKey(provider, model)} {
		x, ok := h.items[key]
		if ok && (x.CooldownUntil.After(now) || x.NeedsProbe) {
			return false
		}
	}
	return true
}
func (h *Health) InCooldown(provider, model string, now time.Time) bool {
	if h == nil {
		return false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	x, ok := h.items[healthKey(provider, model)]
	return ok && x.CooldownUntil.After(now)
}
func (h *Health) DueForProbe(now time.Time) []RouteHealth {
	if h == nil {
		return []RouteHealth{}
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := []RouteHealth{}
	for _, x := range h.items {
		if x.NeedsProbe && !x.CooldownUntil.After(now) {
			out = append(out, x)
		}
	}
	return out
}
func (h *Health) Clear() {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.items = map[string]RouteHealth{}
	h.persistLocked()
	h.mu.Unlock()
}
func (h *Health) Snapshot() []RouteHealth {
	if h == nil {
		return []RouteHealth{}
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]RouteHealth, 0, len(h.items))
	for _, x := range h.items {
		out = append(out, x)
	}
	return out
}
