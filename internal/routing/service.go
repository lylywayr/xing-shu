package routing

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"xing-shu/internal/catalog"
	"xing-shu/internal/provider"
	"xing-shu/internal/quota"
	"xing-shu/internal/runtime"
)

type ProviderConfig struct {
	ID      string
	BaseURL string
	APIKey  string
	Kind    string
}
type Service struct {
	Providers     map[string]ProviderConfig
	ConfigSource  func() map[string]ProviderConfig
	ProviderGate  map[string]func() bool
	QuotaProvider func(string) (quota.Snapshot, bool)
	Models        []catalog.Model
	Manager       interface{ Snapshot() catalog.Catalog }
	Quota         map[string]quota.Snapshot
	Disabled      interface{ IsDisabled(string) bool }
	Client        *provider.ChatClient
	Knowledge     *runtime.KnowledgeStore
	Learning      *runtime.Learning
	Health        *Health
}

func (s *Service) providers() map[string]ProviderConfig {
	if s.ConfigSource != nil {
		return s.ConfigSource()
	}
	return s.Providers
}

func (s *Service) models() []catalog.Model {
	if s.Manager != nil {
		return s.Manager.Snapshot().Models
	}
	return s.Models
}

func (s *Service) HealthSnapshot() []RouteHealth {
	if s.Health == nil {
		return []RouteHealth{}
	}
	return s.Health.Snapshot()
}
func (s *Service) SnapshotModels() []catalog.Model {
	return append([]catalog.Model(nil), s.models()...)
}
func (s *Service) HasModel(name string) bool {
	name = strings.TrimPrefix(name, "openai/")
	providerID := ""
	if slash := strings.Index(name, "/"); slash > 0 {
		providerID, name = name[:slash], name[slash+1:]
	}
	for _, m := range s.models() {
		if m.ID == name && (providerID == "" || m.Provider == providerID) && m.Admitted && m.AutoRoutable && s.HasProvider(m.Provider) {
			return true
		}
	}
	return false
}
func (s *Service) SelectAuto(body []byte) string {
	candidates := s.RankedCandidates(body)
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0].Model
}
func (s *Service) KnowledgeBonus(body []byte, model catalog.Model) int {
	if s.Knowledge == nil || HighRisk(body) {
		return 0
	}
	bonus := 0
	for _, k := range s.Knowledge.All() {
		if k.Status != runtime.KnowledgeTrusted || k.ExpiresAt == nil || !k.ExpiresAt.After(time.Now()) || k.RecommendedModel == "" || k.Signature != runtime.Signature(TaskFeaturesFromBody(body)) {
			continue
		}
		if k.RecommendedModel != model.ID {
			continue
		}
		successRate := 1.0
		if k.ValidatedSamples > 0 {
			successRate = float64(k.SuccessfulSamples) / float64(k.ValidatedSamples)
		}
		sampleWeight := float64(k.ValidatedSamples)
		if sampleWeight > 10 {
			sampleWeight = 10
		}
		fresh := 1.0
		left := time.Until(*k.ExpiresAt).Hours() / 720
		if left < 0.25 {
			fresh = left * 4
		}
		bonus += int(k.Confidence * successRate * fresh * (sampleWeight / 10) * 100)
	}
	return bonus
}

func (s *Service) ProviderFor(name string) string {
	name = strings.TrimPrefix(name, "openai/")
	providerID := ""
	if slash := strings.Index(name, "/"); slash > 0 {
		providerID, name = name[:slash], name[slash+1:]
	}
	for _, m := range s.models() {
		if m.ID == name && (providerID == "" || m.Provider == providerID) {
			return m.Provider
		}
	}
	return ""
}
func ProviderConfigs(configs map[string]provider.Config) map[string]ProviderConfig {
	out := make(map[string]ProviderConfig, len(configs))
	for id, c := range configs {
		out[id] = ProviderConfig{ID: c.ID, BaseURL: c.BaseURL, APIKey: c.APIKey, Kind: c.Kind}
	}
	return out
}
func (s *Service) QuotaExhausted(id string) bool {
	if s.QuotaProvider != nil {
		if q, ok := s.QuotaProvider(id); ok && q.HardExhausted(time.Now()) {
			return true
		}
	}
	if q, ok := s.Quota[id]; ok && q.HardExhausted(time.Now()) {
		return true
	}
	return false
}

func (s *Service) HasProvider(id string) bool {
	if gate, ok := s.ProviderGate[id]; ok && gate != nil && !gate() {
		return false
	}
	if s.Disabled != nil && s.Disabled.IsDisabled(id) {
		return false
	}
	if s.QuotaProvider != nil {
		if q, ok := s.QuotaProvider(id); ok && q.HardExhausted(time.Now()) {
			return false
		}
	}
	if q, ok := s.Quota[id]; ok && q.HardExhausted(time.Now()) {
		return false
	}
	p, ok := s.providers()[id]
	return ok && p.BaseURL != ""
}
func (s *Service) Complete(ctx context.Context, body []byte, model string) (*http.Response, error) {
	var q struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &q) != nil {
		return nil, provider.ErrCatalogEmpty
	}
	if model != "" {
		q.Model = model
	}
	name := strings.TrimPrefix(q.Model, "openai/")
	requestedProvider := ""
	if slash := strings.Index(name, "/"); slash > 0 {
		requestedProvider, name = name[:slash], name[slash+1:]
	}
	if name == "auto" || name == "" {
		name = s.SelectAuto(body)
		if name == "" {
			return nil, provider.ErrCatalogEmpty
		}
		body = replaceModel(body, name)
	}
	var p ProviderConfig
	providers := s.providers()
	for _, m := range s.models() {
		if m.ID == name && m.Admitted && m.AutoRoutable && (requestedProvider == "" || m.Provider == requestedProvider) {
			p = providers[m.Provider]
			break
		}
	}
	if p.ID == "" {
		for _, x := range providers {
			if strings.HasPrefix(name, x.ID+"/") {
				p = x
				break
			}
		}
	}
	if p.ID == "" {
		return nil, provider.ErrCatalogEmpty
	}
	if gate, ok := s.ProviderGate[p.ID]; ok && gate != nil && !gate() {
		return nil, provider.ErrCatalogEmpty
	}
	return s.Client.Complete(ctx, p.BaseURL, p.APIKey, body)
}
func replaceModel(body []byte, name string) []byte {
	var x map[string]any
	if json.Unmarshal(body, &x) != nil {
		return body
	}
	x["model"] = name
	b, _ := json.Marshal(x)
	return b
}
func RequiredCapabilities(body []byte) []string {
	var x struct {
		Tools          []any `json:"tools"`
		ResponseFormat any   `json:"response_format"`
		Messages       []struct {
			Content any `json:"content"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(body, &x)
	needs := []string{}
	if len(x.Tools) > 0 {
		needs = append(needs, "tools")
	}
	if x.ResponseFormat != nil {
		needs = append(needs, "structured_output")
	}
	vision := false
	for _, message := range x.Messages {
		if parts, ok := message.Content.([]any); ok {
			for _, part := range parts {
				if item, ok := part.(map[string]any); ok && (item["type"] == "image_url" || item["type"] == "input_image") {
					vision = true
				}
			}
		}
	}
	if vision {
		needs = append(needs, "vision")
	}
	return needs
}
func Needs(body []byte) string {
	needs := RequiredCapabilities(body)
	if len(needs) == 0 {
		return "general"
	}
	return needs[0]
}
func has(a []string, x string) bool {
	for _, v := range a {
		if v == x {
			return true
		}
	}
	return false
}
func Pick(models []catalog.Model, quotes map[string]quota.Snapshot, need string) []Candidate {
	return Candidates(models, quotes, Need{Capability: need})
}
