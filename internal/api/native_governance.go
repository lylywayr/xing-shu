package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"xing-shu/internal/catalog"
	"xing-shu/internal/governance"
)

type GovernanceRuntime struct {
	Snapshots *governance.Manager
	Catalog   *catalog.Manager
}

func (g GovernanceRuntime) List(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"files": g.Snapshots.List()})
}
func (g GovernanceRuntime) Detail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || g.Snapshots == nil {
		http.Error(w, "snapshots unavailable", http.StatusServiceUnavailable)
		return
	}
	name := filepath.Base(r.URL.Query().Get("name"))
	path := filepath.Join(g.Snapshots.Dir, name)
	b, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, "snapshot not found", http.StatusNotFound)
		return
	}
	hash := sha256.Sum256(b)
	var snapshot catalog.Catalog
	_ = json.Unmarshal(b, &snapshot)
	current := catalog.Catalog{}
	if g.Catalog != nil {
		current = g.Catalog.Snapshot()
	}
	writeJSON(w, map[string]any{"name": name, "sha256": hex.EncodeToString(hash[:]), "bytes": len(b), "models": len(snapshot.Models), "governance": 0, "current_models": len(current.Models), "model_delta": len(current.Models) - len(snapshot.Models), "dry_run_restore": true})
}
func (g GovernanceRuntime) Save(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	if e := g.Snapshots.SaveCatalog(g.Catalog.Snapshot()); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
func (g GovernanceRuntime) Restore(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	name := filepath.Base(r.URL.Query().Get("name"))
	c, e := g.Snapshots.Restore(name)
	if e != nil {
		http.Error(w, "snapshot not found", 404)
		return
	}
	if len(c.Models) == 0 {
		http.Error(w, "empty snapshot", 400)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "name": name, "models": len(c.Models), "dry_run": true})
}
