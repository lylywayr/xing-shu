package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type OpsState struct {
	mu            sync.RWMutex
	Disabled      map[string]bool
	CooldownUntil map[string]time.Time
	SyncErrors    map[string]SyncError
	path          string
}

type SyncError struct {
	Type    string    `json:"type"`
	Message string    `json:"message"`
	At      time.Time `json:"at"`
}

func NewOps() *OpsState {
	return &OpsState{Disabled: map[string]bool{}, CooldownUntil: map[string]time.Time{}, SyncErrors: map[string]SyncError{}}
}
func (o *OpsState) Load(path string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.path = path
	b, e := os.ReadFile(path)
	if e == nil {
		var x struct {
			Disabled      map[string]bool      `json:"disabled"`
			CooldownUntil map[string]time.Time `json:"cooldown_until"`
			SyncErrors    map[string]SyncError `json:"sync_errors"`
		}
		if json.Unmarshal(b, &x) == nil {
			if x.Disabled != nil {
				o.Disabled = x.Disabled
			}
			if x.CooldownUntil != nil {
				o.CooldownUntil = x.CooldownUntil
			}
			if x.SyncErrors != nil {
				o.SyncErrors = x.SyncErrors
			}
		}
	}
}
func (o *OpsState) persist() {
	if o.path == "" {
		return
	}
	o.mu.RLock()
	x := struct {
		Disabled      map[string]bool      `json:"disabled"`
		CooldownUntil map[string]time.Time `json:"cooldown_until"`
		SyncErrors    map[string]SyncError `json:"sync_errors"`
	}{o.Disabled, o.CooldownUntil, o.SyncErrors}
	b, _ := json.Marshal(x)
	o.mu.RUnlock()
	tmp := o.path + ".tmp"
	if os.MkdirAll(filepath.Dir(o.path), 0755) == nil && os.WriteFile(tmp, b, 0644) == nil {
		_ = os.Rename(tmp, o.path)
	}
}
func (o *OpsState) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	o.mu.Lock()
	id := r.URL.Query().Get("id")
	path := r.URL.Path
	switch path {
	case "/api/admin/provider/disable":
		if id == "" {
			o.mu.Unlock()
			http.Error(w, "missing id", 400)
			return
		}
		o.Disabled[id] = true
	case "/api/admin/provider/enable":
		if id == "" {
			o.mu.Unlock()
			http.Error(w, "missing id", 400)
			return
		}
		delete(o.Disabled, id)
	case "/api/admin/cooldowns/clear":
		o.CooldownUntil = map[string]time.Time{}
	default:
		o.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	o.mu.Unlock()
	o.persist()
	writeJSON(w, map[string]any{"ok": true, "action": r.URL.Path, "provider": id})
}
func (o *OpsState) IsDisabled(id string) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.Disabled[id]
}
func (o *OpsState) Cooldown(id string) (time.Time, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	until, ok := o.CooldownUntil[id]
	return until, ok && until.After(time.Now())
}

func (o *OpsState) MarkSyncError(providerID, kind, message string) {
	o.mu.Lock()
	if o.SyncErrors == nil {
		o.SyncErrors = map[string]SyncError{}
	}
	o.SyncErrors[providerID] = SyncError{Type: kind, Message: message, At: time.Now().UTC()}
	o.mu.Unlock()
	o.persist()
}

func (o *OpsState) ClearSyncError(providerID string) {
	o.mu.Lock()
	delete(o.SyncErrors, providerID)
	o.mu.Unlock()
	o.persist()
}

func (o *OpsState) SyncError(providerID string) (SyncError, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	errorState, ok := o.SyncErrors[providerID]
	return errorState, ok
}
