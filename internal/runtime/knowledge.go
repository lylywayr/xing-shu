package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

type KnowledgeStatus string

const (
	KnowledgeProposed  KnowledgeStatus = "proposed"
	KnowledgeValidated KnowledgeStatus = "validated"
	KnowledgeTrusted   KnowledgeStatus = "trusted"
	KnowledgeRejected  KnowledgeStatus = "rejected"
	KnowledgeExpired   KnowledgeStatus = "expired"
)

type FailureCause string

const (
	CauseUnknown    FailureCause = "unknown"
	CauseCapability FailureCause = "capability_insufficient"
	CauseProvider   FailureCause = "provider_failure"
	CauseRateLimit  FailureCause = "rate_limited"
	CauseContext    FailureCause = "context_limit"
	CauseProtocol   FailureCause = "protocol_error"
	CauseUserRetry  FailureCause = "user_retry"
	CauseTeacher    FailureCause = "teacher_error"
)

type KnowledgeValidation struct {
	ID         string          `json:"id"`
	At         time.Time       `json:"at"`
	Success    bool            `json:"success"`
	Cause      FailureCause    `json:"failure_cause,omitempty"`
	Note       string          `json:"note,omitempty"`
	StatusFrom KnowledgeStatus `json:"status_from"`
	StatusTo   KnowledgeStatus `json:"status_to"`
}

type Knowledge struct {
	ID                string                `json:"id"`
	Signature         string                `json:"signature"`
	Features          TaskFeatures          `json:"features"`
	ActualModel       string                `json:"actual_model"`
	ActualProvider    string                `json:"actual_provider"`
	RecommendedModel  string                `json:"recommended_model"`
	MinimumTier       int                   `json:"minimum_tier"`
	Confidence        float64               `json:"confidence"`
	Samples           int                   `json:"samples"`
	ValidatedSamples  int                   `json:"validated_samples"`
	SuccessfulSamples int                   `json:"successful_samples"`
	FailedSamples     int                   `json:"failed_samples"`
	Status            KnowledgeStatus       `json:"status"`
	FailureCause      FailureCause          `json:"failure_cause,omitempty"`
	SourceReviewIDs   []string              `json:"source_review_ids,omitempty"`
	ValidationIDs     []string              `json:"validation_ids,omitempty"`
	ValidationHistory []KnowledgeValidation `json:"validation_history,omitempty"`
	CreatedAt         time.Time             `json:"created_at"`
	UpdatedAt         time.Time             `json:"updated_at"`
	ExpiresAt         *time.Time            `json:"expires_at,omitempty"`
	HumanNote         string                `json:"human_note,omitempty"`
}
type KnowledgeStore struct {
	mu    sync.RWMutex
	items map[string]Knowledge
	path  string
}

func NewKnowledgeStore() *KnowledgeStore { return &KnowledgeStore{items: map[string]Knowledge{}} }
func (s *KnowledgeStore) Load(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.path = path
	b, e := os.ReadFile(path)
	if e == nil {
		var x map[string]Knowledge
		if json.Unmarshal(b, &x) == nil && x != nil {
			s.items = x
		}
	}
	now := time.Now()
	for id, x := range s.items {
		if x.ExpiresAt != nil && x.ExpiresAt.Before(now) && (x.Status == KnowledgeTrusted || x.Status == KnowledgeValidated) {
			x.Status = KnowledgeExpired
			s.items[id] = x
		}
	}
}
func (s *KnowledgeStore) persist() {
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
func (s *KnowledgeStore) All() []Knowledge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Knowledge, 0, len(s.items))
	for _, x := range s.items {
		out = append(out, x)
	}
	return out
}
func (s *KnowledgeStore) Upsert(x Knowledge) {
	if x.ID == "" {
		return
	}
	s.mu.Lock()
	old, ok := s.items[x.ID]
	if ok {
		newSources := 0
		for _, id := range x.SourceReviewIDs {
			seen := false
			for _, oid := range old.SourceReviewIDs {
				if id == oid {
					seen = true
					break
				}
			}
			if !seen {
				old.SourceReviewIDs = append(old.SourceReviewIDs, id)
				newSources++
			}
		}
		x.Samples = old.Samples + newSources
		if x.Samples == 0 {
			x.Samples = 1
		}
		x.ValidatedSamples = old.ValidatedSamples
		x.SuccessfulSamples = old.SuccessfulSamples
		x.FailedSamples = old.FailedSamples
		x.ValidationIDs = old.ValidationIDs
		x.ValidationHistory = old.ValidationHistory
		x.Status = old.Status
		x.FailureCause = old.FailureCause
		x.HumanNote = old.HumanNote
		x.ExpiresAt = old.ExpiresAt
		if x.CreatedAt.IsZero() {
			x.CreatedAt = old.CreatedAt
		}
	}
	s.items[x.ID] = x
	s.mu.Unlock()
	s.persist()
}
func (s *KnowledgeStore) Apply(id string, fn func(*Knowledge)) bool {
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
func (s *KnowledgeStore) RecordValidation(id, validationID string, success bool, cause FailureCause, note string) bool {
	if validationID == "" {
		return false
	}
	s.mu.Lock()
	k, ok := s.items[id]
	if !ok {
		s.mu.Unlock()
		return false
	}
	for _, v := range k.ValidationIDs {
		if v == validationID {
			s.mu.Unlock()
			return false
		}
	}
	before := k.Status
	k.ValidationIDs = append(k.ValidationIDs, validationID)
	k.ValidatedSamples++
	if success {
		k.SuccessfulSamples++
	} else {
		k.FailedSamples++
		k.FailureCause = cause
	}
	if k.ValidatedSamples >= 3 && k.SuccessfulSamples*100 >= k.ValidatedSamples*80 && k.Status == KnowledgeProposed {
		k.Status = KnowledgeValidated
	}
	if k.ValidatedSamples >= 10 && k.SuccessfulSamples*100 >= k.ValidatedSamples*90 && k.Status == KnowledgeValidated {
		k.Status = KnowledgeTrusted
	}
	if note != "" {
		k.HumanNote = note
	}
	if k.ExpiresAt == nil {
		expires := time.Now().Add(30 * 24 * time.Hour)
		k.ExpiresAt = &expires
	}
	k.UpdatedAt = time.Now()
	k.ValidationHistory = append(k.ValidationHistory, KnowledgeValidation{ID: validationID, At: time.Now().UTC(), Success: success, Cause: cause, Note: note, StatusFrom: before, StatusTo: k.Status})
	s.items[id] = k
	s.mu.Unlock()
	s.persist()
	return true
}
func (s *KnowledgeStore) ChangeStatus(id string, status KnowledgeStatus, note string, cause FailureCause) bool {
	if status != KnowledgeProposed && status != KnowledgeValidated && status != KnowledgeTrusted && status != KnowledgeRejected && status != KnowledgeExpired {
		return false
	}
	s.mu.Lock()
	k, ok := s.items[id]
	if !ok || k.Status == status {
		s.mu.Unlock()
		return false
	}
	before := k.Status
	k.Status, k.HumanNote, k.FailureCause, k.UpdatedAt = status, note, cause, time.Now().UTC()
	k.ValidationHistory = append(k.ValidationHistory, KnowledgeValidation{ID: "decision-" + k.UpdatedAt.Format("20060102150405.000000000"), At: k.UpdatedAt, Success: status != KnowledgeRejected && status != KnowledgeExpired, Cause: cause, Note: note, StatusFrom: before, StatusTo: status})
	s.items[id] = k
	s.mu.Unlock()
	s.persist()
	return true
}
func (s *KnowledgeStore) Restore(id string, status KnowledgeStatus, note string) bool {
	if status != KnowledgeValidated && status != KnowledgeTrusted {
		return false
	}
	s.mu.Lock()
	k, ok := s.items[id]
	if !ok || (k.Status != KnowledgeExpired && k.Status != KnowledgeRejected) {
		s.mu.Unlock()
		return false
	}
	before := k.Status
	now := time.Now()
	expires := now.Add(30 * 24 * time.Hour)
	k.Status, k.HumanNote, k.ExpiresAt, k.UpdatedAt = status, note, &expires, now
	k.ValidationHistory = append(k.ValidationHistory, KnowledgeValidation{ID: "restore-" + now.UTC().Format("20060102150405.000000000"), At: now.UTC(), Success: true, Note: note, StatusFrom: before, StatusTo: status})
	s.items[id] = k
	s.mu.Unlock()
	s.persist()
	return true
}
func (s *KnowledgeStore) Expire(now time.Time) int {
	n := 0
	s.mu.Lock()
	for id, k := range s.items {
		if k.ExpiresAt != nil && k.ExpiresAt.Before(now) && (k.Status == KnowledgeTrusted || k.Status == KnowledgeValidated) {
			k.Status = KnowledgeExpired
			k.UpdatedAt = now
			s.items[id] = k
			n++
		}
	}
	s.mu.Unlock()
	if n > 0 {
		s.persist()
	}
	return n
}
func Signature(f TaskFeatures) string {
	return f.Domain + "|" + f.Action + "|tools=" + boolString(f.NeedsTools) + "|vision=" + boolString(f.NeedsVision) + "|json=" + boolString(f.NeedsStructured) + "|reason=" + strconv.Itoa(f.ReasoningLevel)
}
func boolString(x bool) string {
	if x {
		return "1"
	}
	return "0"
}
