package capability

import (
	"context"
	"time"
	"xing-shu/internal/catalog"
)

type ProbeResult struct {
	Capabilities map[string]catalog.Evidence `json:"capabilities"`
	Error        string                      `json:"error,omitempty"`
	CheckedAt    time.Time                   `json:"checked_at"`
}
type Prober interface {
	Probe(context.Context, catalog.Model) (ProbeResult, error)
}

func Apply(m *catalog.Model, r ProbeResult) {
	if m.CapabilityEvidence == nil {
		m.CapabilityEvidence = map[string]catalog.Evidence{}
	}
	for k, v := range r.Capabilities {
		m.CapabilityEvidence[k] = v
		if v.Supported {
			switch k {
			case "tools":
				m.Tools = true
			case "vision":
				m.Vision = true
			case "structured_output":
				m.StructuredOutput = true
			case "parallel_tools":
				m.ParallelTools = true
			case "thinking_blocks":
				m.ThinkingBlocks = true
			}
		} else {
			switch k {
			case "tools":
				m.Tools = false
			case "vision":
				m.Vision = false
			case "structured_output":
				m.StructuredOutput = false
			case "parallel_tools":
				m.ParallelTools = false
			case "thinking_blocks":
				m.ThinkingBlocks = false
			}
		}
	}
	m.UpdatedAt = time.Now()
}
func Supports(m catalog.Model, need string) bool {
	if e, ok := m.CapabilityEvidence[need]; ok {
		return e.Supported
	}
	switch need {
	case "tools":
		return m.Tools
	case "vision":
		return m.Vision
	case "structured_output":
		return m.StructuredOutput
	case "parallel_tools":
		return m.ParallelTools
	case "thinking_blocks":
		return m.ThinkingBlocks
	}
	for _, x := range m.Capabilities {
		if x == need {
			return true
		}
	}
	return false
}
