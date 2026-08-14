package catalog

import (
	"time"
)

// RestoreReviewerCapability rehydrates a previously verified teacher selection
// after a catalog sync or process restart. It only applies to an active Auto model.
func (m *Manager) RestoreReviewerCapability(modelID, providerID string) bool {
	return m.UpdateProviderModel(modelID, providerID, func(model *Model) {
		if model.Status != Active || !model.AutoRoutable {
			return
		}
		model.StructuredOutput = true
		model.StructuredOutputKnown = true
		if model.CapabilityEvidence == nil {
			model.CapabilityEvidence = map[string]Evidence{}
		}
		model.CapabilityEvidence["structured_output"] = Evidence{Supported: true, Confidence: 1, Source: "persisted reviewer selection", Level: "selection", CheckedAt: time.Now()}
	})
}
