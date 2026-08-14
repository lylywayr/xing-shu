package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type ReviewerConfig struct {
	Model               string     `json:"model"`
	Provider            string     `json:"provider"`
	SelectedAt          *time.Time `json:"selected_at,omitempty"`
	LastCheckedAt       *time.Time `json:"last_checked_at,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	LastError           string     `json:"last_error,omitempty"`
}
type ReviewerSelection struct {
	mu     sync.RWMutex
	Config ReviewerConfig
	path   string
}

func NewReviewerSelection() *ReviewerSelection { return &ReviewerSelection{} }
func (s *ReviewerSelection) Load(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.path = path
	b, e := os.ReadFile(path)
	if e == nil {
		_ = json.Unmarshal(b, &s.Config)
	}
}
func (s *ReviewerSelection) persist() {
	s.mu.RLock()
	b, _ := json.Marshal(s.Config)
	path := s.path
	s.mu.RUnlock()
	if path == "" {
		return
	}
	tmp := path + ".tmp"
	if os.MkdirAll(filepath.Dir(path), 0755) == nil && os.WriteFile(tmp, b, 0600) == nil {
		_ = os.Rename(tmp, path)
	}
}
func (s *ReviewerSelection) Get() ReviewerConfig { s.mu.RLock(); defer s.mu.RUnlock(); return s.Config }
func (s *ReviewerSelection) Set(model, provider string, session *ReviewSession) {
	now := time.Now()
	s.mu.Lock()
	old := s.Config
	s.Config = ReviewerConfig{Model: model, Provider: provider, SelectedAt: &now}
	s.mu.Unlock()
	s.persist()
	if session != nil && (old.Model != "" || old.Provider != "") && (old.Model != model || old.Provider != provider) {
		session.Handoff(old.Model, old.Provider, model, provider, "用户更换教师模型")
	}
}
func (s *ReviewerSelection) Clear(reason string, session *ReviewSession) {
	s.mu.Lock()
	old := s.Config
	s.Config = ReviewerConfig{LastError: reason}
	s.mu.Unlock()
	s.persist()
	if session != nil && old.Model != "" {
		session.Handoff(old.Model, old.Provider, "未选择", "", reason)
	}
}
func (s *ReviewerSelection) Available() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Config.Model != ""
}
func (s *ReviewerSelection) Failure(err string, threshold int, session *ReviewSession) {
	s.mu.Lock()
	s.Config.ConsecutiveFailures++
	s.Config.LastError = err
	s.Config.LastCheckedAt = func() *time.Time { x := time.Now(); return &x }()
	clearNow := threshold > 0 && s.Config.ConsecutiveFailures >= threshold
	s.mu.Unlock()
	if clearNow {
		s.Clear(err, session)
	} else {
		s.persist()
	}
}
func (s *ReviewerSelection) Success() {
	s.mu.Lock()
	s.Config.ConsecutiveFailures = 0
	s.Config.LastError = ""
	x := time.Now()
	s.Config.LastCheckedAt = &x
	s.mu.Unlock()
	s.persist()
}
