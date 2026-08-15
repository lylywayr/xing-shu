package catalog

import (
	"context"
	"strings"
	"time"
	"xing-shu/internal/provider"
)

const FreeLLMAPIProviderID = "freellmapi"

// IsAutoApprovedProvider identifies providers whose complete catalog is trusted
// to enter the routable pool without per-model user approval.
func IsAutoApprovedProvider(providerID string) bool { return providerID == FreeLLMAPIProviderID }

type SyncService struct {
	Allow    map[string]bool
	Admitted map[string]bool
}

func (s SyncService) Apply(providerID string, raw []provider.RawModel) []Model {
	out := make([]Model, 0, len(raw))
	for _, x := range raw {
		id := strings.TrimSpace(x.ID)
		if id == "" || len(id) > 300 || strings.ContainsAny(id, "\r\n\x00") || IsMeta(id) {
			continue
		}
		if x.Context < 0 || x.Context > 10_000_000 {
			x.Context = 0
		}
		key := providerID + "/" + x.ID
		admitted := s.Admitted[key] || IsAutoApprovedProvider(providerID)
		m := Model{ID: id, Provider: providerID, Object: "model", OwnedBy: providerID, Created: time.Now().Unix(), ContextWindow: x.Context, Tools: x.Tools, Vision: x.Vision, StructuredOutput: x.StructuredOutput || x.JSONMode, StructuredOutputKnown: x.StructuredOutputKnown || x.StructuredOutput || x.JSONMode, Status: Unknown, Admitted: admitted, UpdatedAt: time.Now()}
		approved := s.Allow[key]
		m.AutoRoutable = !IsMeta(m.ID) && IsAutoApprovedProvider(providerID) || (!IsMeta(m.ID) && admitted && approved)
		if admitted || m.AutoRoutable {
			m.Status = Active
		}
		out = append(out, m)
	}
	return out
}
func IsMeta(id string) bool {
	for _, x := range []string{"auto", "fusion", "compound", "router", "routing", "subagent", "sub_agent", "browser_use", "fallback", "aggregator"} {
		if id == x {
			return true
		}
	}
	return false
}
func SyncOne(ctx context.Context, c provider.Config, allow map[string]bool) ([]Model, provider.Result) {
	raw, r := provider.FetchModels(ctx, c)
	if r.ErrorType != "" {
		return nil, r
	}
	return SyncService{Allow: allow}.Apply(c.ID, raw), r
}
func Version() string { return time.Now().UTC().Format("20060102T150405Z") }
