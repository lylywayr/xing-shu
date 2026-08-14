package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	Dir string
	mu  sync.RWMutex
}

func New(dir string) *Store { _ = os.MkdirAll(dir, 0700); return &Store{Dir: dir} }
func (s *Store) ReadJSON(name string, v any) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, e := os.ReadFile(filepath.Join(s.Dir, name))
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func (s *Store) WriteJSON(name string, v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp := filepath.Join(s.Dir, name+".tmp")
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	return os.Rename(tmp, filepath.Join(s.Dir, name))
}
