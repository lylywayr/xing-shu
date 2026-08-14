package governance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
	"xing-shu/internal/catalog"
)

type Manager struct{ Dir string }

func NewManager(dir string) *Manager { _ = os.MkdirAll(dir, 0700); return &Manager{Dir: dir} }
func (m *Manager) SaveCatalog(c catalog.Catalog) error {
	b, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	h := sha256.Sum256(b)
	name := time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(h[:])[:12] + ".json"
	return os.WriteFile(filepath.Join(m.Dir, name), b, 0600)
}
func (m *Manager) List() []string {
	fs, _ := filepath.Glob(filepath.Join(m.Dir, "*.json"))
	out := []string{}
	for _, x := range fs {
		out = append(out, filepath.Base(x))
	}
	return out
}
func (m *Manager) Restore(name string) (catalog.Catalog, error) {
	var c catalog.Catalog
	b, e := os.ReadFile(filepath.Join(m.Dir, filepath.Base(name)))
	if e != nil {
		return c, e
	}
	e = json.Unmarshal(b, &c)
	return c, e
}
