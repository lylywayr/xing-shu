package routing

import (
	"sort"
	"xing-shu/internal/capability"
	"xing-shu/internal/catalog"
	"xing-shu/internal/quota"
)

type Need struct {
	Capability string
	Provider   string
}
type Candidate struct {
	Model   catalog.Model
	Score   float64
	Reasons []string
}

func Candidates(models []catalog.Model, q map[string]quota.Snapshot, n Need) []Candidate {
	out := []Candidate{}
	for _, m := range models {
		if !m.AutoRoutable || m.Status != catalog.Active {
			continue
		}
		if n.Provider != "" && m.Provider != n.Provider {
			continue
		}
		if n.Capability != "" && !capability.Supports(m, n.Capability) {
			continue
		}
		if x, ok := q[m.Provider]; ok && x.State != quota.Available {
			continue
		}
		out = append(out, Candidate{Model: m, Score: float64(m.Score), Reasons: []string{"auto_allow", "status_active", "capability_match", "quota_available"}})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}
