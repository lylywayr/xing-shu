package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type ReviewMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type ReviewSession struct {
	mu          sync.RWMutex
	Summary     string          `json:"summary"`
	Messages    []ReviewMessage `json:"messages"`
	Turns       int             `json:"turns"`
	Compactions int             `json:"compactions"`
	UpdatedAt   time.Time       `json:"updated_at"`
	path        string
}

func NewReviewSession() *ReviewSession { return &ReviewSession{Messages: []ReviewMessage{}} }
func (s *ReviewSession) Load(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.path = path
	b, e := os.ReadFile(path)
	if e == nil {
		_ = json.Unmarshal(b, s)
		s.path = path
		if s.Messages == nil {
			s.Messages = []ReviewMessage{}
		}
	}
}
func (s *ReviewSession) persist() {
	s.mu.RLock()
	b, _ := json.Marshal(struct {
		Summary     string          `json:"summary"`
		Messages    []ReviewMessage `json:"messages"`
		Turns       int             `json:"turns"`
		Compactions int             `json:"compactions"`
		UpdatedAt   time.Time       `json:"updated_at"`
	}{s.Summary, append([]ReviewMessage(nil), s.Messages...), s.Turns, s.Compactions, s.UpdatedAt})
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
func (s *ReviewSession) Snapshot() ([]ReviewMessage, string, int, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]ReviewMessage(nil), s.Messages...), s.Summary, s.Turns, s.Compactions
}
func (s *ReviewSession) Handoff(fromModel, fromProvider, toModel, toProvider, reason string) {
	content := "教师交接：上一位教师=" + fromModel + "（" + fromProvider + "），当前教师=" + toModel + "（" + toProvider + "）。原因=" + reason + "。请继续使用本会话的长期复盘记忆，不要清空历史。"
	s.mu.Lock()
	s.Messages = append(s.Messages, ReviewMessage{"system", content})
	s.Turns++
	s.UpdatedAt = time.Now()
	s.mu.Unlock()
	s.persist()
}
func (s *ReviewSession) Append(user, assistant string) {
	s.mu.Lock()
	if user != "" {
		s.Messages = append(s.Messages, ReviewMessage{"user", user})
	}
	if assistant != "" {
		s.Messages = append(s.Messages, ReviewMessage{"assistant", assistant})
	}
	s.Turns++
	s.UpdatedAt = time.Now()
	s.mu.Unlock()
	s.persist()
}
func (s *ReviewSession) Replace(summary string, msgs []ReviewMessage) {
	s.mu.Lock()
	s.Summary = summary
	s.Messages = append([]ReviewMessage(nil), msgs...)
	s.Compactions++
	s.UpdatedAt = time.Now()
	s.mu.Unlock()
	s.persist()
}
func (s *ReviewSession) ApproxChars() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := len(s.Summary)
	for _, m := range s.Messages {
		n += len(m.Role) + len(m.Content)
	}
	return n
}
