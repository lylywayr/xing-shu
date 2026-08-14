package governance

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
	"xing-shu/internal/catalog"
)

type Service struct{ Records map[string]Record }

func New() *Service { return &Service{Records: map[string]Record{}} }
func (s *Service) Upsert(m catalog.Model, reason string) {
	key := m.Provider + "/" + m.ID
	h := sha256.Sum256([]byte(key + string(m.Status) + reason))
	s.Records[key] = Record{Key: key, Model: m, Status: m.Status, Reason: reason, EvidenceHash: hex.EncodeToString(h[:]), CheckedAt: time.Now(), LastSeen: time.Now()}
}
