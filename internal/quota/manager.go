package quota

import (
	"sync"
	"time"
)

type Manager struct {
	mu      sync.RWMutex
	items   map[string]Snapshot
	history []Snapshot
}

func NewManager() *Manager { return &Manager{items: map[string]Snapshot{}} }
func snapshotKey(x Snapshot) string {
	return x.Provider + "\x00" + x.Pool + "\x00" + x.Metric
}

func (m *Manager) Set(x Snapshot) {
	if m == nil || x.Provider == "" {
		return
	}
	m.mu.Lock()
	if m.items == nil {
		m.items = map[string]Snapshot{}
	}
	m.items[snapshotKey(x)] = x
	m.history = append(m.history, x)
	if len(m.history) > 200 {
		m.history = m.history[len(m.history)-200:]
	}
	m.mu.Unlock()
}
func (m *Manager) History() []Snapshot {
	if m == nil {
		return []Snapshot{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]Snapshot(nil), m.history...)
}
func (m *Manager) All() []Snapshot {
	if m == nil {
		return []Snapshot{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	o := make([]Snapshot, 0, len(m.items))
	for _, x := range m.items {
		o = append(o, x)
	}
	return o
}
func (m *Manager) GetMetric(provider, metric string) (Snapshot, bool) {
	if m == nil {
		return Snapshot{}, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var found Snapshot
	ok := false
	for _, x := range m.items {
		if x.Provider == provider && x.Metric == metric && (!ok || x.CheckedAt.After(found.CheckedAt)) {
			found, ok = x, true
		}
	}
	return found, ok
}

func (m *Manager) Get(provider string) (Snapshot, bool) {
	if m == nil {
		return Snapshot{}, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var found Snapshot
	ok := false
	for _, x := range m.items {
		if x.Provider == provider {
			if !ok || x.CheckedAt.After(found.CheckedAt) {
				found = x
				ok = true
			}
		}
	}
	return found, ok
}
func (m *Manager) Unknown(provider string) Snapshot {
	return Snapshot{Provider: provider, State: Unknown, CheckedAt: time.Now()}
}
