package catalog

import "time"

type Status string

const (
	Active   Status = "active"
	Unknown  Status = "unknown"
	Meta     Status = "meta"
	Stale    Status = "stale"
	Retired  Status = "retired"
	Disabled Status = "disabled"
)

type Evidence struct {
	Supported  bool      `json:"supported"`
	Confidence float64   `json:"confidence"`
	Source     string    `json:"source"`
	Level      string    `json:"level"`
	URL        string    `json:"url"`
	Hash       string    `json:"hash"`
	CheckedAt  time.Time `json:"checked_at"`
}
type Model struct {
	ID                    string              `json:"id"`
	Provider              string              `json:"provider"`
	Object                string              `json:"object"`
	OwnedBy               string              `json:"owned_by"`
	Created               int64               `json:"created"`
	Status                Status              `json:"status"`
	AutoRoutable          bool                `json:"auto_routable"`
	Capabilities          []string            `json:"capabilities"`
	ContextWindow         int                 `json:"context_window"`
	Tools                 bool                `json:"tools"`
	Vision                bool                `json:"vision"`
	StructuredOutput      bool                `json:"structured_output"`
	StructuredOutputKnown bool                `json:"structured_output_known"`
	ParallelTools         bool                `json:"parallel_tools"`
	ThinkingBlocks        bool                `json:"thinking_blocks"`
	CapabilityEvidence    map[string]Evidence `json:"capability_evidence,omitempty"`
	Score                 int                 `json:"score"`
	UpdatedAt             time.Time           `json:"updated_at"`
}
type Catalog struct {
	Models   []Model   `json:"models"`
	SyncedAt time.Time `json:"synced_at"`
	Version  string    `json:"version"`
}
