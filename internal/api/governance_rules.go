package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"xing-shu/internal/catalog"
)

type GovernanceRuleChange struct {
	Key   string `json:"key"`
	Allow bool   `json:"allow"`
}
type governanceRuleRequest struct {
	Changes  []GovernanceRuleChange `json:"changes"`
	Revision string                 `json:"revision"`
}
type GovernanceRuleAudit struct {
	Revision string                 `json:"revision"`
	Action   string                 `json:"action"`
	Changes  []GovernanceRuleChange `json:"changes"`
	Previous []GovernanceRuleChange `json:"previous,omitempty"`
	At       time.Time              `json:"at"`
}

type GovernanceRulesRuntime struct {
	Manager *catalog.Manager
	dir     string
	mu      sync.RWMutex
	rules   map[string]bool
	audits  []GovernanceRuleAudit
}

func NewGovernanceRulesRuntime(manager *catalog.Manager, dir string) *GovernanceRulesRuntime {
	r := &GovernanceRulesRuntime{Manager: manager, dir: dir, rules: manager.AllowSnapshot()}
	r.load()
	return r
}
func (r *GovernanceRulesRuntime) validate(changes []GovernanceRuleChange) error {
	if len(changes) == 0 || len(changes) > 100 {
		return errors.New("changes must contain 1 to 100 items")
	}
	seen := map[string]bool{}
	for _, change := range changes {
		parts := strings.SplitN(change.Key, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" || seen[change.Key] {
			return fmt.Errorf("invalid or duplicate model key: %s", change.Key)
		}
		seen[change.Key] = true
		found := false
		for _, model := range r.Manager.Snapshot().Models {
			if model.Provider == parts[0] && model.ID == parts[1] {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("model not found: %s", change.Key)
		}
	}
	return nil
}
func (r *GovernanceRulesRuntime) Preview(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body governanceRuleRequest
	if json.NewDecoder(http.MaxBytesReader(w, req.Body, 128*1024)).Decode(&body) != nil {
		http.Error(w, "invalid JSON body", 400)
		return
	}
	if err := r.validate(body.Changes); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	r.mu.RLock()
	current := make(map[string]bool, len(r.rules))
	for key, value := range r.rules {
		current[key] = value
	}
	r.mu.RUnlock()
	changes := make([]map[string]any, 0, len(body.Changes))
	for _, change := range body.Changes {
		changes = append(changes, map[string]any{"key": change.Key, "before": current[change.Key], "after": change.Allow, "changed": current[change.Key] != change.Allow})
	}
	writeJSON(w, map[string]any{"ok": true, "changes": changes, "count": len(changes), "dry_run": true})
}
func (r *GovernanceRulesRuntime) Apply(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body governanceRuleRequest
	if json.NewDecoder(http.MaxBytesReader(w, req.Body, 128*1024)).Decode(&body) != nil {
		http.Error(w, "invalid JSON body", 400)
		return
	}
	if err := r.validate(body.Changes); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	r.mu.Lock()
	revision := fmt.Sprintf("gov-%d", time.Now().UnixNano())
	previous := make([]GovernanceRuleChange, 0, len(body.Changes))
	for _, change := range body.Changes {
		previous = append(previous, GovernanceRuleChange{Key: change.Key, Allow: r.rules[change.Key]})
		r.rules[change.Key] = change.Allow
	}
	audit := GovernanceRuleAudit{Revision: revision, Action: "apply", Changes: append([]GovernanceRuleChange(nil), body.Changes...), Previous: append([]GovernanceRuleChange(nil), previous...), At: time.Now().UTC()}
	r.audits = append(r.audits, audit)
	r.mu.Unlock()
	r.applyManager(body.Changes)
	r.save()
	writeJSON(w, map[string]any{"ok": true, "revision": revision, "changed": body.Changes, "previous": previous})
}
func (r *GovernanceRulesRuntime) Undo(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body governanceRuleRequest
	if json.NewDecoder(http.MaxBytesReader(w, req.Body, 16*1024)).Decode(&body) != nil || body.Revision == "" {
		http.Error(w, "revision is required", 400)
		return
	}
	r.mu.Lock()
	var target *GovernanceRuleAudit
	for i := len(r.audits) - 1; i >= 0; i-- {
		if r.audits[i].Revision == body.Revision && r.audits[i].Action == "apply" {
			x := r.audits[i]
			target = &x
			break
		}
	}
	r.mu.Unlock()
	if target == nil {
		http.Error(w, "revision not found", 404)
		return
	}
	changes := append([]GovernanceRuleChange(nil), target.Previous...)
	if err := r.validate(changes); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	r.mu.Lock()
	for _, change := range changes {
		r.rules[change.Key] = change.Allow
	}
	revision := fmt.Sprintf("undo-%d", time.Now().UnixNano())
	r.audits = append(r.audits, GovernanceRuleAudit{Revision: revision, Action: "undo", Changes: changes, At: time.Now().UTC()})
	r.mu.Unlock()
	r.applyManager(changes)
	r.save()
	writeJSON(w, map[string]any{"ok": true, "revision": revision, "undone": body.Revision})
}
func (r *GovernanceRulesRuntime) Audit() []GovernanceRuleAudit {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]GovernanceRuleAudit(nil), r.audits...)
}

func (r *GovernanceRulesRuntime) AuditView(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"items": r.Audit()})
}
func (r *GovernanceRulesRuntime) applyManager(changes []GovernanceRuleChange) {
	for _, change := range changes {
		parts := strings.SplitN(change.Key, "/", 2)
		r.Manager.SetAllow(parts[0], parts[1], change.Allow)
	}
}
func (r *GovernanceRulesRuntime) load() {
	if r.dir == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(r.dir, "governance-rules-xing-shu.json"))
	if err != nil {
		return
	}
	var state struct {
		Rules  map[string]bool       `json:"rules"`
		Audits []GovernanceRuleAudit `json:"audits"`
	}
	if json.Unmarshal(data, &state) == nil {
		if state.Rules != nil {
			r.rules = state.Rules
		}
		r.audits = state.Audits
	}
	changes := make([]GovernanceRuleChange, 0, len(r.rules))
	for key, allow := range r.rules {
		changes = append(changes, GovernanceRuleChange{Key: key, Allow: allow})
	}
	r.applyManager(changes)
}
func (r *GovernanceRulesRuntime) save() {
	if r.dir == "" {
		return
	}
	r.mu.RLock()
	state := struct {
		Rules  map[string]bool       `json:"rules"`
		Audits []GovernanceRuleAudit `json:"audits"`
	}{r.rules, r.audits}
	data, _ := json.MarshalIndent(state, "", "  ")
	r.mu.RUnlock()
	_ = os.MkdirAll(r.dir, 0700)
	tmp := filepath.Join(r.dir, "governance-rules-xing-shu.json.tmp")
	if os.WriteFile(tmp, data, 0600) == nil {
		_ = os.Rename(tmp, filepath.Join(r.dir, "governance-rules-xing-shu.json"))
	}
}
