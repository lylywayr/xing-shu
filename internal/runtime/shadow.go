package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type ShadowStatus string

const (
	ShadowPending   ShadowStatus = "pending"
	ShadowRunning   ShadowStatus = "running"
	ShadowCompleted ShadowStatus = "completed"
	ShadowSkipped   ShadowStatus = "skipped"
	ShadowFailed    ShadowStatus = "failed"
)

type ShadowResult struct {
	ID                string       `json:"id"`
	KnowledgeID       string       `json:"knowledge_id"`
	ReviewID          string       `json:"review_id"`
	BatchID           string       `json:"batch_id,omitempty"`
	CandidateModel    string       `json:"candidate_model"`
	Status            ShadowStatus `json:"status"`
	HTTPStatus        int          `json:"http_status"`
	LatencyMS         int64        `json:"latency_ms"`
	ProtocolOK        bool         `json:"protocol_ok"`
	StructuredOK      bool         `json:"structured_ok"`
	ToolCompatible    bool         `json:"tool_compatible"`
	SemanticMatch     bool         `json:"semantic_match"`
	QualityConclusion string       `json:"quality_conclusion"`
	Success           bool         `json:"success"`
	Cause             FailureCause `json:"failure_cause"`
	Error             string       `json:"error,omitempty"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
}
type ShadowStore struct {
	mu    sync.RWMutex
	items map[string]ShadowResult
	path  string
}

func NewShadowStore() *ShadowStore { return &ShadowStore{items: map[string]ShadowResult{}} }
func (s *ShadowStore) Load(p string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.path = p
	b, e := os.ReadFile(p)
	if e == nil {
		var x map[string]ShadowResult
		if json.Unmarshal(b, &x) == nil && x != nil {
			s.items = x
		}
	}
}
func (s *ShadowStore) persist() {
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
func (s *ShadowStore) All() []ShadowResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	o := make([]ShadowResult, 0, len(s.items))
	for _, x := range s.items {
		o = append(o, x)
	}
	return o
}
func (s *ShadowStore) Put(x ShadowResult) { s.mu.Lock(); s.items[x.ID] = x; s.mu.Unlock(); s.persist() }
func (s *ShadowStore) Has(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.items[id]
	return ok
}
func (s *ShadowStore) Apply(id string, fn func(*ShadowResult)) bool {
	s.mu.Lock()
	x, ok := s.items[id]
	if ok {
		fn(&x)
		x.UpdatedAt = time.Now()
		s.items[id] = x
	}
	s.mu.Unlock()
	if ok {
		s.persist()
	}
	return ok
}
