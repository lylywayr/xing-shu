package catalog

import (
	"context"
	"net/http"
	"sync"
	"time"
	"xing-shu/internal/provider"
)

type Manager struct {
	mu       sync.RWMutex
	Current  Catalog
	Allow    map[string]bool
	Admitted map[string]bool
	last     map[string]time.Time
	persist  func(State)
}

func NewManager(c Catalog, allow map[string]bool) *Manager {
	return NewManagerWithState(State{Catalog: c, Allow: allow})
}

func NewManagerWithState(state State) *Manager {
	if state.Allow == nil {
		state.Allow = map[string]bool{}
	}
	if state.Admitted == nil {
		state.Admitted = map[string]bool{}
		for key, allowed := range state.Allow {
			if allowed {
				state.Admitted[key] = true
				delete(state.Allow, key)
			}
		}
	}
	if state.Catalog.Models == nil {
		state.Catalog.Models = []Model{}
	}
	last := map[string]time.Time{}
	for key, value := range state.Last {
		last[key] = value
	}
	return &Manager{Current: state.Catalog, Allow: state.Allow, Admitted: state.Admitted, last: last}
}

func (m *Manager) SetPersist(fn func(State)) { m.mu.Lock(); m.persist = fn; m.mu.Unlock() }

func (m *Manager) persistLocked() {
	if m.persist == nil {
		return
	}
	state := State{Catalog: m.Current, Allow: m.Allow, Admitted: m.Admitted, Last: m.last}
	state.Catalog.Models = append([]Model(nil), m.Current.Models...)
	state.Allow = make(map[string]bool, len(m.Allow))
	for key, value := range m.Allow {
		state.Allow[key] = value
	}
	state.Admitted = make(map[string]bool, len(m.Admitted))
	for key, value := range m.Admitted {
		state.Admitted[key] = value
	}
	state.Last = make(map[string]time.Time, len(m.last))
	for key, value := range m.last {
		state.Last[key] = value
	}
	m.persist(state)
}
func (m *Manager) Snapshot() Catalog {
	m.mu.RLock()
	defer m.mu.RUnlock()
	x := m.Current
	x.Models = append([]Model(nil), x.Models...)
	return x
}
func (m *Manager) Sync(ctx context.Context, c provider.Config) provider.Result {
	raw, res := provider.FetchModels(ctx, c)
	if res.ErrorType != "" {
		return res
	}
	return m.SyncRaw(c.ID, raw)
}

func (m *Manager) SyncRaw(providerID string, raw []provider.RawModel) provider.Result {
	res := provider.Result{Status: http.StatusOK, LastSuccess: time.Now()}
	allow := m.AllowSnapshot()
	admitted := m.AdmissionSnapshot()
	next := SyncService{Allow: allow, Admitted: admitted}.Apply(providerID, raw)
	m.mu.Lock()
	defer m.mu.Unlock()
	previous := map[string]Model{}
	for _, model := range m.Current.Models {
		if model.Provider == providerID {
			previous[model.ID] = model
		}
	}
	for i := range next {
		if oldModel, ok := previous[next[i].ID]; ok {
			next[i].Orphaned = false
			next[i].StructuredOutput = oldModel.StructuredOutput
			next[i].StructuredOutputKnown = oldModel.StructuredOutputKnown
			next[i].CapabilityEvidence = oldModel.CapabilityEvidence
			next[i].Tools = oldModel.Tools || next[i].Tools
			next[i].Vision = oldModel.Vision || next[i].Vision
		}
	}
	old := 0
	for _, x := range m.Current.Models {
		if x.Provider == providerID {
			old++
		}
	}
	if old >= 10 && len(next)*2 < old {
		res.ErrorType = provider.InvalidCatalog
		res.Message = "catalog drop protected"
		return res
	}
	keep := []Model{}
	for _, x := range m.Current.Models {
		if x.Provider != providerID {
			keep = append(keep, x)
		}
	}
	m.Current.Models = append(keep, next...)
	m.Current.SyncedAt = time.Now()
	m.Current.Version = Version()
	m.last[providerID] = time.Now()
	m.persistLocked()
	return res
}
func (m *Manager) Last(providerID string) time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.last[providerID]
}
func (m *Manager) MarkSynced(providerID string, at time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.last[providerID] = at
	m.persistLocked()
}
func (m *Manager) UpdateProviderModel(id, providerID string, fn func(*Model)) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Current.Models {
		if m.Current.Models[i].ID == id && m.Current.Models[i].Provider == providerID {
			fn(&m.Current.Models[i])
			m.Current.Version = Version()
			m.persistLocked()
			return true
		}
	}
	return false
}
func (m *Manager) UpdateModel(id string, fn func(*Model)) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Current.Models {
		if m.Current.Models[i].ID == id {
			fn(&m.Current.Models[i])
			m.Current.Version = Version()
			m.persistLocked()
			return true
		}
	}
	return false
}

// ApplyDefaultApprovalPolicy migrates existing catalog entries to the current
// two-layer approval policy. It never revives stale or orphaned models.
func (m *Manager) ApplyDefaultApprovalPolicy() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	changed := 0
	for i := range m.Current.Models {
		model := &m.Current.Models[i]
		if IsMeta(model.ID) || model.Status == Stale || model.Orphaned {
			continue
		}
		key := model.Provider + "/" + model.ID
		modelChanged := false
		if IsAutoApprovedProvider(model.Provider) {
			if model.Status == Unknown {
				model.Status = Active
				modelChanged = true
			}
			if !model.Admitted {
				model.Admitted = true
				modelChanged = true
			}
			if model.Status == Active && !model.AutoRoutable {
				model.AutoRoutable = true
				modelChanged = true
			}
		} else {
			admitted := m.Admitted[key] || model.Admitted
			if admitted && model.Status == Unknown {
				model.Status = Active
				modelChanged = true
			}
			if model.Admitted != admitted {
				model.Admitted = admitted
				modelChanged = true
			}
			if model.AutoRoutable {
				model.AutoRoutable = false
				modelChanged = true
			}
		}
		if modelChanged {
			model.UpdatedAt = time.Now().UTC()
			changed++
		}
	}
	if changed > 0 {
		m.Current.Version = Version()
		m.persistLocked()
	}
	return changed
}

func (m *Manager) SetAdmitted(providerID, modelID string, admitted bool) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Admitted == nil {
		m.Admitted = map[string]bool{}
	}
	key := providerID + "/" + modelID
	m.Admitted[key] = admitted
	if !admitted && m.Allow != nil {
		m.Allow[key] = false
	}
	changed := false
	for i := range m.Current.Models {
		model := &m.Current.Models[i]
		if model.Provider != providerID || model.ID != modelID || IsAutoApprovedProvider(providerID) {
			continue
		}
		model.Admitted = admitted
		model.AutoRoutable = false
		if admitted && model.Status == Unknown && !IsMeta(modelID) {
			model.Status = Active
		}
		if !admitted && model.Status == Active {
			model.Status = Unknown
		}
		model.UpdatedAt = time.Now().UTC()
		changed = true
	}
	if changed {
		m.Current.Version = Version()
		m.persistLocked()
	}
	return changed
}

func AutoApprovalReady(model Model) bool {
	if IsAutoApprovedProvider(model.Provider) {
		return true
	}
	if !model.Admitted || model.Status != Active || IsMeta(model.ID) {
		return false
	}
	for _, capability := range []string{"structured_output", "tools", "vision"} {
		evidence, ok := model.CapabilityEvidence[capability]
		if !ok || evidence.Source != "runtime_probe" || evidence.Level != "protocol" {
			return false
		}
	}
	return true
}

func (m *Manager) SetAllow(providerID, modelID string, allowed bool) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Allow == nil {
		m.Allow = map[string]bool{}
	}
	for i := range m.Current.Models {
		model := &m.Current.Models[i]
		if model.Provider != providerID || model.ID != modelID {
			continue
		}
		if allowed && !AutoApprovalReady(*model) {
			return false
		}
		key := providerID + "/" + modelID
		m.Allow[key] = allowed
		model.AutoRoutable = allowed && model.Status == Active
		model.UpdatedAt = time.Now().UTC()
		m.Current.Version = Version()
		m.persistLocked()
		return true
	}
	return false
}

func (m *Manager) ReconcileProviders(configs map[string]provider.Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	changed := false
	for i := range m.Current.Models {
		model := &m.Current.Models[i]
		_, online := configs[model.Provider]
		if !online {
			if model.Status != Stale || model.AutoRoutable || !model.Orphaned {
				model.Status = Stale
				model.AutoRoutable = false
				model.Orphaned = true
				model.UpdatedAt = time.Now().UTC()
				changed = true
			}
			continue
		}
		if model.Status == Stale && model.Orphaned {
			model.Status = Unknown
			model.AutoRoutable = false
			model.Orphaned = false
			model.UpdatedAt = time.Now().UTC()
			changed = true
		}
	}
	if changed {
		m.Current.Version = Version()
		m.persistLocked()
	}
}

func (m *Manager) AdmissionSnapshot() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]bool, len(m.Admitted))
	for key, admitted := range m.Admitted {
		out[key] = admitted
	}
	return out
}

func (m *Manager) AllowSnapshot() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]bool, len(m.Allow))
	for key, allowed := range m.Allow {
		out[key] = allowed
	}
	return out
}
