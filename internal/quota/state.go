package quota

import "time"

type State string

const (
	Available    State = "available"
	Exhausted    State = "exhausted"
	Unknown      State = "unknown"
	NotSupported State = "not_supported"
)

type Snapshot struct {
	Provider   string     `json:"provider"`
	Pool       string     `json:"pool,omitempty"`
	Metric     string     `json:"metric,omitempty"`
	Unit       string     `json:"unit,omitempty"`
	State      State      `json:"state"`
	Remaining  *float64   `json:"remaining,omitempty"`
	Used       *float64   `json:"used,omitempty"`
	Limit      *float64   `json:"limit,omitempty"`
	ResetAt    *time.Time `json:"reset_at,omitempty"`
	CheckedAt  time.Time  `json:"checked_at"`
	FreshUntil *time.Time `json:"fresh_until,omitempty"`
	Source     string     `json:"source,omitempty"`
	Confidence float64    `json:"confidence,omitempty"`
	ErrorType  string     `json:"error_type,omitempty"`
}

func (s Snapshot) Fresh(now time.Time) bool         { return s.FreshUntil == nil || now.Before(*s.FreshUntil) }
func (s Snapshot) HardExhausted(now time.Time) bool { return s.State == Exhausted && s.Fresh(now) }

func HasFacts(items []Snapshot) bool {
	for _, item := range items {
		if item.State == Available || item.State == Exhausted {
			return true
		}
	}
	return false
}
