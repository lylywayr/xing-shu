package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Session struct {
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	UID       string    `json:"uid"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Sessions struct {
	mu    sync.RWMutex
	items map[string]Session
	TTL   time.Duration
	path  string
}

func NewSessions() *Sessions { return &Sessions{items: map[string]Session{}, TTL: 2 * time.Hour} }
func (s *Sessions) Load(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.path = path
	b, e := os.ReadFile(path)
	if e == nil {
		var x map[string]Session
		if json.Unmarshal(b, &x) == nil {
			now := time.Now()
			for k, v := range x {
				if now.Sub(v.UpdatedAt) < s.TTL {
					s.items[k] = v
				}
			}
		}
	}
}
func (s *Sessions) persist() {
	if s.path == "" {
		return
	}
	b, _ := json.Marshal(s.items)
	tmp := s.path + ".tmp"
	if os.MkdirAll(filepath.Dir(s.path), 0755) == nil && os.WriteFile(tmp, b, 0644) == nil {
		_ = os.Rename(tmp, s.path)
	}
}
func (s *Sessions) Get(id string) (Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	x, ok := s.items[id]
	return x, ok && time.Since(x.UpdatedAt) < s.TTL
}
func (s *Sessions) Put(id string, x Session) {
	if id == "" {
		return
	}
	s.mu.Lock()
	x.UpdatedAt = time.Now()
	s.items[id] = x
	s.mu.Unlock()
	s.persist()
}
