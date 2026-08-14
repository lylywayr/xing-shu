package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type ShadowBatchStatus string

const (
	ShadowBatchPending   ShadowBatchStatus = "pending"
	ShadowBatchRunning   ShadowBatchStatus = "running"
	ShadowBatchCompleted ShadowBatchStatus = "completed"
	ShadowBatchStopped   ShadowBatchStatus = "stopped"
	ShadowBatchFailed    ShadowBatchStatus = "failed"
)

type ShadowBatch struct {
	ID             string            `json:"id"`
	KnowledgeID    string            `json:"knowledge_id"`
	ReviewIDs      []string          `json:"review_ids"`
	CandidateModel string            `json:"candidate_model"`
	Status         ShadowBatchStatus `json:"status"`
	Total          int               `json:"total"`
	Completed      int               `json:"completed"`
	Succeeded      int               `json:"succeeded"`
	Failed         int               `json:"failed"`
	Stopped        int               `json:"stopped"`
	LastError      string            `json:"last_error,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

type ShadowBatches struct {
	mu    sync.RWMutex
	items map[string]ShadowBatch
	path  string
}

func NewShadowBatches() *ShadowBatches { return &ShadowBatches{items: map[string]ShadowBatch{}} }
func (s *ShadowBatches) Load(path string) {
	s.mu.Lock()
	s.deferPersistLocked(path)
	s.path = path
	b, e := os.ReadFile(path)
	if e == nil {
		var x map[string]ShadowBatch
		if json.Unmarshal(b, &x) == nil && x != nil {
			s.items = x
		}
	}
	now := time.Now().UTC()
	for id, item := range s.items {
		if item.Status == ShadowBatchPending || item.Status == ShadowBatchRunning {
			item.Status = ShadowBatchStopped
			item.LastError = "batch recovered after process restart"
			item.UpdatedAt = now
			s.items[id] = item
		}
	}
	s.mu.Unlock()
	s.persist()
}
func (s *ShadowBatches) deferPersistLocked(_ string) {}
func (s *ShadowBatches) RecoverInterrupted() int {
	n := 0
	s.mu.Lock()
	for id, item := range s.items {
		if item.Status == ShadowBatchPending || item.Status == ShadowBatchRunning {
			item.Status = ShadowBatchStopped
			item.LastError = "batch recovered after process restart"
			item.UpdatedAt = time.Now().UTC()
			s.items[id] = item
			n++
		}
	}
	s.mu.Unlock()
	if n > 0 {
		s.persist()
	}
	return n
}
func (s *ShadowBatches) persist() {
	s.mu.RLock()
	b, _ := json.Marshal(s.items)
	p := s.path
	s.mu.RUnlock()
	if p == "" {
		return
	}
	tmp := p + ".tmp"
	if os.MkdirAll(filepath.Dir(p), 0755) == nil && os.WriteFile(tmp, b, 0600) == nil {
		_ = os.Rename(tmp, p)
	}
}
func (s *ShadowBatches) Put(x ShadowBatch) {
	if x.ID == "" {
		return
	}
	s.mu.Lock()
	s.items[x.ID] = x
	s.mu.Unlock()
	s.persist()
}
func (s *ShadowBatches) Get(id string) (ShadowBatch, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	x, ok := s.items[id]
	return x, ok
}
func (s *ShadowBatches) All() []ShadowBatch {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ShadowBatch, 0, len(s.items))
	for _, x := range s.items {
		out = append(out, x)
	}
	return out
}
func (s *ShadowBatches) Apply(id string, fn func(*ShadowBatch)) bool {
	s.mu.Lock()
	x, ok := s.items[id]
	if ok {
		fn(&x)
		x.UpdatedAt = time.Now().UTC()
		s.items[id] = x
	}
	s.mu.Unlock()
	if ok {
		s.persist()
	}
	return ok
}
func (s *ShadowBatches) Stop(id string) bool {
	return s.Apply(id, func(x *ShadowBatch) {
		if x.Status == ShadowBatchPending || x.Status == ShadowBatchRunning {
			x.Status = ShadowBatchStopped
			remaining := x.Total - x.Completed
			if remaining > 0 {
				x.Stopped += remaining
			}
		}
	})
}
func (s *ShadowBatches) Retry(id string) bool {
	return s.Apply(id, func(x *ShadowBatch) {
		if x.Status == ShadowBatchFailed || x.Status == ShadowBatchStopped {
			x.Status = ShadowBatchPending
			x.Completed, x.Succeeded, x.Failed, x.Stopped = 0, 0, 0, 0
			x.LastError = ""
		}
	})
}
