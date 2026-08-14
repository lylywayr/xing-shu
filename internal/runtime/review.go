package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type ReviewStatus string

const (
	StatusPending    ReviewStatus = "pending"
	StatusProcessing ReviewStatus = "processing"
	StatusReviewed   ReviewStatus = "reviewed"
	StatusFailed     ReviewStatus = "failed"
	StatusSkipped    ReviewStatus = "skipped"
)

type TaskFeatures struct {
	Domain           string `json:"domain"`
	Action           string `json:"action"`
	NeedsTools       bool   `json:"needs_tools"`
	NeedsVision      bool   `json:"needs_vision"`
	NeedsStructured  bool   `json:"needs_structured"`
	NeedsLongContext bool   `json:"needs_long_context"`
	ReasoningLevel   int    `json:"reasoning_level"`
	MessageCount     int    `json:"message_count"`
	InputBytes       int    `json:"input_bytes"`
}
type Review struct {
	ID                       string       `json:"id"`
	RecordedAt               time.Time    `json:"recorded_at"`
	Model                    string       `json:"model"`
	Provider                 string       `json:"provider"`
	Status                   int          `json:"status"`
	LatencyMS                int64        `json:"latency_ms"`
	Stream                   bool         `json:"stream"`
	Tools                    bool         `json:"tools"`
	Features                 TaskFeatures `json:"features"`
	ReviewStatus             ReviewStatus `json:"review_status"`
	BatchID                  string       `json:"batch_id,omitempty"`
	Attempts                 int          `json:"attempts"`
	ReviewerModel            string       `json:"reviewer_model,omitempty"`
	ReviewedAt               *time.Time   `json:"reviewed_at,omitempty"`
	RecommendedModel         string       `json:"recommended_model,omitempty"`
	MinimumTier              int          `json:"minimum_tier,omitempty"`
	Overqualified            bool         `json:"overqualified,omitempty"`
	Confidence               float64      `json:"confidence,omitempty"`
	Conclusion               string       `json:"conclusion,omitempty"`
	ReviewError              string       `json:"review_error,omitempty"`
	Replayable               bool         `json:"replayable"`
	ReplayReason             string       `json:"replay_reason,omitempty"`
	TaskPackage              []byte       `json:"task_package,omitempty"`
	TaskPackageEncrypted     bool         `json:"task_package_encrypted"`
	ResponsePackage          []byte       `json:"response_package,omitempty"`
	ResponseCaptured         bool         `json:"response_captured"`
	ResponsePackageEncrypted bool         `json:"response_package_encrypted"`
}
type Reviews struct {
	mu     sync.RWMutex
	items  map[string]Review
	path   string
	locked bool
}

func NewReviews() *Reviews { return &Reviews{items: map[string]Review{}} }
func (r *Reviews) Load(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.path = path
	b, e := os.ReadFile(path)
	if e == nil {
		var x map[string]Review
		if json.Unmarshal(b, &x) == nil {
			r.items = x
		}
		for k, x := range r.items {
			if x.ReviewStatus == "" {
				x.ReviewStatus = StatusPending
				r.items[k] = x
			}
		}
	}
}
func (r *Reviews) persist() {
	if r.path == "" {
		return
	}
	r.mu.RLock()
	b, _ := json.Marshal(r.items)
	r.mu.RUnlock()
	tmp := r.path + ".tmp"
	if os.MkdirAll(filepath.Dir(r.path), 0755) == nil && os.WriteFile(tmp, b, 0600) == nil {
		_ = os.Rename(tmp, r.path)
	}
}
func (r *Reviews) Add(x Review) {
	if x.ID == "" {
		return
	}
	if x.ReviewStatus == "" {
		x.ReviewStatus = StatusPending
	}
	r.mu.Lock()
	r.items[x.ID] = x
	r.mu.Unlock()
	r.persist()
}
func (r *Reviews) Find(id string) (Review, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	x, ok := r.items[id]
	return x, ok
}
func (r *Reviews) All() []Review {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Review, 0, len(r.items))
	for _, x := range r.items {
		out = append(out, x)
	}
	return out
}
func (r *Reviews) RecoverProcessing(maxAge time.Duration) int {
	now := time.Now()
	count := 0
	r.mu.Lock()
	for id, x := range r.items {
		if x.ReviewStatus == StatusProcessing && (maxAge <= 0 || now.Sub(x.RecordedAt) > maxAge) {
			x.ReviewStatus = StatusFailed
			x.ReviewError = "processing batch recovered after interruption"
			r.items[id] = x
			count++
		}
	}
	r.mu.Unlock()
	if count > 0 {
		r.persist()
	}
	return count
}
func (r *Reviews) Pending(before time.Time) []Review {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []Review{}
	for _, x := range r.items {
		if x.RecordedAt.Before(before) && (x.ReviewStatus == StatusPending || x.ReviewStatus == StatusFailed) {
			out = append(out, x)
		}
	}
	return out
}
func (r *Reviews) Claim(before time.Time, batch string, limit int) []Review {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.locked {
		return nil
	}
	r.locked = true
	out := []Review{}
	for id, x := range r.items {
		if len(out) >= limit {
			break
		}
		if x.RecordedAt.Before(before) && (x.ReviewStatus == StatusPending || x.ReviewStatus == StatusFailed) {
			x.ReviewStatus = StatusProcessing
			x.BatchID = batch
			x.Attempts++
			r.items[id] = x
			out = append(out, x)
		}
	}
	go func() { r.persist(); r.mu.Lock(); r.locked = false; r.mu.Unlock() }()
	return out
}
func (r *Reviews) Apply(id string, fn func(*Review)) {
	r.mu.Lock()
	if x, ok := r.items[id]; ok {
		fn(&x)
		r.items[id] = x
	}
	r.mu.Unlock()
	r.persist()
}
