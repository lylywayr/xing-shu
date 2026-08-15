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

type ModelAdmissionChange struct {
	Key      string `json:"key"`
	Admitted bool   `json:"admitted"`
}
type modelAdmissionRequest struct {
	Changes  []ModelAdmissionChange `json:"changes"`
	Revision string                 `json:"revision"`
}
type ModelAdmissionAudit struct {
	Revision string                 `json:"revision"`
	Action   string                 `json:"action"`
	Changes  []ModelAdmissionChange `json:"changes"`
	Previous []ModelAdmissionChange `json:"previous,omitempty"`
	At       time.Time              `json:"at"`
}
type ModelAdmissionRuntime struct {
	Manager *catalog.Manager
	dir     string
	mu      sync.RWMutex
	audits  []ModelAdmissionAudit
}

func NewModelAdmissionRuntime(manager *catalog.Manager, dir string) *ModelAdmissionRuntime {
	r := &ModelAdmissionRuntime{Manager: manager, dir: dir}
	r.load()
	return r
}
func (r *ModelAdmissionRuntime) validate(changes []ModelAdmissionChange) error {
	if r == nil || r.Manager == nil {
		return errors.New("model admission unavailable")
	}
	if len(changes) == 0 || len(changes) > 100 {
		return errors.New("changes must contain 1 to 100 items")
	}
	models := map[string]catalog.Model{}
	for _, model := range r.Manager.Snapshot().Models {
		models[model.Provider+"/"+model.ID] = model
	}
	seen := map[string]bool{}
	for _, change := range changes {
		if seen[change.Key] {
			return fmt.Errorf("duplicate model key: %s", change.Key)
		}
		seen[change.Key] = true
		model, ok := models[change.Key]
		if !ok || catalog.IsAutoApprovedProvider(model.Provider) || catalog.IsMeta(model.ID) || model.Status == catalog.Stale || model.Orphaned {
			return fmt.Errorf("model cannot be admitted: %s", change.Key)
		}
	}
	return nil
}
func (r *ModelAdmissionRuntime) Preview(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body modelAdmissionRequest
	if json.NewDecoder(http.MaxBytesReader(w, req.Body, 128*1024)).Decode(&body) != nil {
		http.Error(w, "invalid JSON body", 400)
		return
	}
	if err := r.validate(body.Changes); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	admitted := r.Manager.AdmissionSnapshot()
	changes := make([]map[string]any, 0, len(body.Changes))
	for _, change := range body.Changes {
		before := admitted[change.Key]
		changes = append(changes, map[string]any{"key": change.Key, "before": before, "after": change.Admitted, "changed": before != change.Admitted})
	}
	writeJSON(w, map[string]any{"ok": true, "changes": changes, "count": len(changes), "dry_run": true})
}
func (r *ModelAdmissionRuntime) Apply(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body modelAdmissionRequest
	if json.NewDecoder(http.MaxBytesReader(w, req.Body, 128*1024)).Decode(&body) != nil {
		http.Error(w, "invalid JSON body", 400)
		return
	}
	if err := r.validate(body.Changes); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	previous := make([]ModelAdmissionChange, 0, len(body.Changes))
	admitted := r.Manager.AdmissionSnapshot()
	for _, change := range body.Changes {
		previous = append(previous, ModelAdmissionChange{Key: change.Key, Admitted: admitted[change.Key]})
		parts := strings.SplitN(change.Key, "/", 2)
		r.Manager.SetAdmitted(parts[0], parts[1], change.Admitted)
	}
	r.mu.Lock()
	revision := fmt.Sprintf("admit-%d", time.Now().UnixNano())
	r.audits = append(r.audits, ModelAdmissionAudit{Revision: revision, Action: "apply", Changes: append([]ModelAdmissionChange(nil), body.Changes...), Previous: previous, At: time.Now().UTC()})
	r.mu.Unlock()
	r.save()
	writeJSON(w, map[string]any{"ok": true, "revision": revision, "changed": body.Changes, "previous": previous})
}
func (r *ModelAdmissionRuntime) Undo(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body modelAdmissionRequest
	if json.NewDecoder(http.MaxBytesReader(w, req.Body, 16*1024)).Decode(&body) != nil || body.Revision == "" {
		http.Error(w, "revision is required", 400)
		return
	}
	r.mu.RLock()
	var target *ModelAdmissionAudit
	for i := len(r.audits) - 1; i >= 0; i-- {
		if r.audits[i].Revision == body.Revision && r.audits[i].Action == "apply" {
			copy := r.audits[i]
			target = &copy
			break
		}
	}
	r.mu.RUnlock()
	if target == nil {
		http.Error(w, "revision not found", 404)
		return
	}
	if err := r.validate(target.Previous); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	for _, change := range target.Previous {
		parts := strings.SplitN(change.Key, "/", 2)
		r.Manager.SetAdmitted(parts[0], parts[1], change.Admitted)
	}
	r.mu.Lock()
	r.audits = append(r.audits, ModelAdmissionAudit{Revision: fmt.Sprintf("admit-undo-%d", time.Now().UnixNano()), Action: "undo", Changes: append([]ModelAdmissionChange(nil), target.Previous...), At: time.Now().UTC()})
	r.mu.Unlock()
	r.save()
	writeJSON(w, map[string]any{"ok": true, "undone": body.Revision})
}
func (r *ModelAdmissionRuntime) Audit(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	writeJSON(w, map[string]any{"items": append([]ModelAdmissionAudit(nil), r.audits...)})
}
func (r *ModelAdmissionRuntime) load() {
	if r.dir == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(r.dir, "model-admissions-xing-shu.json"))
	if err == nil {
		_ = json.Unmarshal(data, &r.audits)
	}
}
func (r *ModelAdmissionRuntime) save() {
	if r.dir == "" {
		return
	}
	r.mu.RLock()
	data, err := json.MarshalIndent(r.audits, "", "  ")
	r.mu.RUnlock()
	if err != nil {
		return
	}
	_ = os.MkdirAll(r.dir, 0700)
	tmp := filepath.Join(r.dir, "model-admissions-xing-shu.json.tmp")
	if os.WriteFile(tmp, data, 0600) == nil {
		_ = os.Rename(tmp, filepath.Join(r.dir, "model-admissions-xing-shu.json"))
	}
}
