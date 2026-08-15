package api

import (
	"net/http"
	"xing-shu/internal/catalog"
)

type reviewerCandidate struct {
	ID                    string                      `json:"id"`
	Provider              string                      `json:"provider"`
	Status                string                      `json:"status"`
	AutoRoutable          bool                        `json:"auto_routable"`
	StructuredOutput      bool                        `json:"structured_output"`
	StructuredOutputKnown bool                        `json:"structured_output_known"`
	Capabilities          []string                    `json:"capabilities"`
	Tools                 bool                        `json:"tools"`
	Vision                bool                        `json:"vision"`
	ContextWindow         int                         `json:"context_window"`
	Evidence              map[string]catalog.Evidence `json:"capability_evidence,omitempty"`
}

func ReviewerCandidatesView(manager *catalog.Manager, ops *OpsState, _ *Runtime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || manager == nil {
			http.Error(w, "reviewer candidates unavailable", http.StatusServiceUnavailable)
			return
		}
		items := make([]reviewerCandidate, 0)
		probe := make([]reviewerCandidate, 0)
		reasons := map[string]int{}
		total := 0
		for _, model := range manager.Snapshot().Models {
			total++
			if ops != nil && ops.IsDisabled(model.Provider) {
				reasons["provider_disabled"]++
				continue
			}
			candidate := reviewerCandidateFrom(model)
			if model.Status != catalog.Active {
				reasons["not_active"]++
				continue
			}
			if !model.AutoRoutable {
				reasons["second_approval_pending"]++
				continue
			}
			if model.StructuredOutputKnown && model.StructuredOutput {
				items = append(items, candidate)
				continue
			}
			if !model.StructuredOutputKnown {
				probe = append(probe, candidate)
				reasons["structured_output_unknown"]++
				continue
			}
			reasons["structured_output_unsupported"]++
		}
		writeJSON(w, map[string]any{
			"items":                      items,
			"probe_candidates":           probe,
			"eligible":                   len(items),
			"probeable":                  len(probe),
			"excluded":                   total - len(items),
			"reasons":                    reasons,
			"capability_probe_available": true,
		})
	}
}

func reviewerCandidateFrom(model catalog.Model) reviewerCandidate {
	return reviewerCandidate{ID: model.ID, Provider: model.Provider, Status: string(model.Status), AutoRoutable: model.AutoRoutable, StructuredOutput: model.StructuredOutput, StructuredOutputKnown: model.StructuredOutputKnown, Capabilities: model.Capabilities, Tools: model.Tools, Vision: model.Vision, ContextWindow: model.ContextWindow, Evidence: model.CapabilityEvidence}
}

func reasonsTotal(reasons map[string]int, eligible, probeable int) map[string]int {
	out := make(map[string]int, len(reasons)+1)
	for key, value := range reasons {
		out[key] = value
	}
	out["eligible"] = eligible
	out["probeable"] = probeable
	return out
}
