package governance

import (
	"time"
	"xing-shu/internal/catalog"
)

type Record struct {
	Key          string             `json:"key"`
	Model        catalog.Model      `json:"model"`
	Status       catalog.Status     `json:"status"`
	Reason       string             `json:"reason"`
	Evidence     []catalog.Evidence `json:"evidence"`
	EvidenceHash string             `json:"evidence_hash"`
	Probe        map[string]string  `json:"probe,omitempty"`
	MissingSyncs int                `json:"missing_syncs"`
	CheckedAt    time.Time          `json:"checked_at"`
	LastSeen     time.Time          `json:"last_seen"`
}
type Snapshot struct {
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
	SHA256    string    `json:"sha256"`
	Models    int       `json:"models"`
	Entries   int       `json:"entries"`
}
type Consistency struct {
	Catalog    int  `json:"catalog"`
	Governance int  `json:"governance"`
	Missing    int  `json:"missing"`
	Unexpected int  `json:"unexpected"`
	OK         bool `json:"ok"`
}
