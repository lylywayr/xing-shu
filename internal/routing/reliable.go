package routing

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"xing-shu/internal/capability"
	"xing-shu/internal/catalog"
	"xing-shu/internal/provider"
)

type RankedCandidate struct {
	Provider       string         `json:"provider"`
	Model          string         `json:"model"`
	Score          int            `json:"score"`
	Reasons        []string       `json:"reasons"`
	ScoreBreakdown map[string]int `json:"score_breakdown"`
}
type Attempt struct {
	Provider  string      `json:"provider"`
	Model     string      `json:"model"`
	Status    int         `json:"status,omitempty"`
	ErrorType FailureKind `json:"error_type,omitempty"`
	Error     string      `json:"error,omitempty"`
	TTFBMS    int64       `json:"ttfb_ms,omitempty"`
	LatencyMS int64       `json:"latency_ms"`
	Retryable bool        `json:"retryable"`
}
type ReliableOptions struct {
	MaxAttempts       int
	PerAttemptTimeout time.Duration
}
type ReliableResult struct {
	Response *http.Response
	Provider string
	Model    string
	Attempts []Attempt
}

func (s *Service) providerHealthAvailable(provider string, now time.Time) bool {
	if s.Health == nil {
		return true
	}
	for _, item := range s.Health.Snapshot() {
		if item.Provider == provider && item.Model == "*" && (item.CooldownUntil.After(now) || item.NeedsProbe) {
			return false
		}
	}
	return true
}
func (s *Service) RouteAvailable(provider, model string, now time.Time) bool {
	return s.providerHealthAvailable(provider, now) && (s.Health == nil || s.Health.Available(provider, model, now))
}
func (s *Service) RankedCandidates(body []byte) []RankedCandidate {
	needs := RequiredCapabilities(body)
	now := time.Now()
	out := []RankedCandidate{}
	for _, m := range s.models() {
		if !m.Admitted || !m.AutoRoutable || m.Status != catalog.Active || !s.HasProvider(m.Provider) {
			continue
		}
		capabilitiesMatch := true
		for _, need := range needs {
			if !capability.Supports(m, need) {
				capabilitiesMatch = false
				break
			}
		}
		if !capabilitiesMatch {
			continue
		}
		if !s.RouteAvailable(m.Provider, m.ID, now) {
			continue
		}
		base := m.Score
		knowledge := s.KnowledgeBonus(body, m)
		learning := s.learningBonus(m.Provider, m.ID)
		health := 0
		out = append(out, RankedCandidate{Provider: m.Provider, Model: m.ID, Score: base + knowledge + learning + health, Reasons: []string{"admitted", "auto_approved", "provider_available", "capability_match", "health_available"}, ScoreBreakdown: map[string]int{"base_score": base, "knowledge_bonus": knowledge, "learning_bonus": learning, "health_adjustment": health}})
	}
	sortRanked(out)
	return out
}
func sortRanked(items []RankedCandidate) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && (items[j].Score > items[j-1].Score || (items[j].Score == items[j-1].Score && items[j].Provider+"/"+items[j].Model < items[j-1].Provider+"/"+items[j-1].Model)); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
func (s *Service) learningBonus(providerID, model string) int {
	if s.Learning == nil {
		return 0
	}
	stats := s.Learning.Snapshot()
	stat, ok := stats[providerID+"/"+model]
	if !ok {
		stat, ok = stats[model]
	}
	if !ok || stat.Requests == 0 {
		return 0
	}
	success := int(float64(stat.Success) / float64(stat.Requests) * 30)
	avg := stat.LatencyMS / stat.Requests
	latency := 10
	if avg > 1000 {
		latency = 5
	}
	if avg > 5000 {
		latency = 0
	}
	return success + latency
}
func failureFor(status int, err error) (FailureKind, bool) {
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return FailureTimeout, true
		}
		return FailureNetwork, true
	}
	switch {
	case status == 401 || status == 403:
		return FailureAuth, true
	case status == 408 || status == 425:
		return FailureTimeout, true
	case status == 429:
		return FailureRateLimited, true
	case status >= 500:
		return FailureServer, true
	case status >= 400:
		return FailureProtocol, false
	}
	return "", false
}
func retryAfter(resp *http.Response, now time.Time) time.Time {
	if resp == nil {
		return time.Time{}
	}
	v := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if n, e := strconv.Atoi(v); e == nil && n > 0 {
		return now.Add(time.Duration(n) * time.Second)
	}
	if t, e := http.ParseTime(v); e == nil {
		return t
	}
	return time.Time{}
}
func validateSuccess(resp *http.Response, stream bool) (*http.Response, error) {
	if resp == nil {
		return nil, errors.New("empty upstream response")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, nil
	}
	if stream {
		limited := &io.LimitedReader{R: resp.Body, N: 64 << 10}
		reader := bufio.NewReader(limited)
		var prefix bytes.Buffer
		valid := false
		for prefix.Len() < 64<<10 {
			line, err := reader.ReadString('\n')
			prefix.WriteString(line)
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "data:") || strings.HasPrefix(trimmed, "event:") {
				valid = true
			}
			if valid && trimmed == "" {
				break
			}
			if err != nil {
				if err != io.EOF {
					resp.Body.Close()
					return resp, err
				}
				break
			}
		}
		if !valid {
			resp.Body.Close()
			return resp, errors.New("invalid SSE response before first event")
		}
		resp.Body = &multiReadCloser{Reader: io.MultiReader(bytes.NewReader(prefix.Bytes()), reader, resp.Body), Closer: resp.Body}
		resp.ContentLength = -1
		return resp, nil
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	resp.Body.Close()
	if err != nil {
		return resp, err
	}
	var payload any
	if json.Unmarshal(b, &payload) != nil {
		resp.Body = io.NopCloser(bytes.NewReader(b))
		return resp, errors.New("invalid JSON response")
	}
	resp.Body = io.NopCloser(bytes.NewReader(b))
	resp.ContentLength = int64(len(b))
	return resp, nil
}

type multiReadCloser struct {
	io.Reader
	io.Closer
}
type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelReadCloser) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}
func (s *Service) CompleteReliable(ctx context.Context, body []byte, options ReliableOptions) (*ReliableResult, error) {
	if s.Client == nil {
		s.Client = provider.NewChatClient()
	}
	if s.Health == nil {
		s.Health = NewHealth(HealthOptions{})
	}
	if options.MaxAttempts <= 0 {
		options.MaxAttempts = 3
	}
	if options.PerAttemptTimeout <= 0 {
		options.PerAttemptTimeout = 45 * time.Second
	}
	var flags struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if json.Unmarshal(body, &flags) != nil {
		return nil, errors.New("invalid request JSON")
	}
	candidates := s.RankedCandidates(body)
	if flags.Model != "" && flags.Model != "auto" {
		requested := strings.TrimPrefix(flags.Model, "openai/")
		requestedProvider := ""
		requestedModel := requested
		if slash := strings.Index(requested, "/"); slash > 0 {
			requestedProvider, requestedModel = requested[:slash], requested[slash+1:]
		}
		filtered := []RankedCandidate{}
		for _, c := range candidates {
			if c.Model == requestedModel && (requestedProvider == "" || c.Provider == requestedProvider) {
				filtered = append(filtered, c)
			}
		}
		candidates = filtered
	}
	if len(candidates) == 0 {
		return nil, provider.ErrCatalogEmpty
	}
	result := &ReliableResult{Attempts: []Attempt{}}
	skippedProviders := map[string]bool{}
	var last error
	for _, candidate := range candidates {
		if len(result.Attempts) >= options.MaxAttempts {
			break
		}
		if skippedProviders[candidate.Provider] {
			continue
		}
		started := time.Now()
		attemptCtx, cancel := context.WithTimeout(ctx, options.PerAttemptTimeout)
		cfg := s.providers()[candidate.Provider]
		resp, err := s.Client.Complete(attemptCtx, cfg.BaseURL, cfg.APIKey, replaceModel(body, candidate.Model))
		if err != nil {
			cancel()
		} else if resp != nil {
			resp.Body = &cancelReadCloser{ReadCloser: resp.Body, cancel: cancel}
		} else {
			cancel()
		}
		status := 0
		ttfb := time.Since(started).Milliseconds()
		if resp != nil {
			status = resp.StatusCode
		}
		kind, retryable := failureFor(status, err)
		if resp == nil && err == nil {
			err = errors.New("empty upstream response")
			kind, retryable = FailureProtocol, true
		}
		if err == nil && status >= 200 && status < 300 {
			resp, err = validateSuccess(resp, flags.Stream)
			if err != nil {
				kind = FailureProtocol
				retryable = true
			}
		}
		attempt := Attempt{Provider: candidate.Provider, Model: candidate.Model, Status: status, TTFBMS: ttfb, LatencyMS: time.Since(started).Milliseconds(), ErrorType: kind, Retryable: retryable}
		if err != nil {
			attempt.Error = err.Error()
		}
		result.Attempts = append(result.Attempts, attempt)
		if s.Learning != nil {
			s.Learning.Record(candidate.Provider+"/"+candidate.Model, err == nil && status >= 200 && status < 300, time.Since(started))
		}
		if err == nil && status >= 200 && status < 300 {
			s.Health.Success(candidate.Provider, candidate.Model, time.Now())
			result.Response = resp
			result.Provider = candidate.Provider
			result.Model = candidate.Model
			return result, nil
		}
		if resp != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
			resp.Body.Close()
		}
		if kind != "" {
			scopeModel := candidate.Model
			if kind == FailureNetwork || kind == FailureTimeout || kind == FailureRateLimited || kind == FailureServer || kind == FailureAuth {
				scopeModel = "*"
				skippedProviders[candidate.Provider] = true
			}
			if kind == FailureRateLimited {
				s.Health.FailUntil(candidate.Provider, scopeModel, kind, time.Now(), retryAfter(resp, time.Now()))
			} else {
				s.Health.Fail(candidate.Provider, scopeModel, kind, time.Now())
			}
		}
		if !retryable {
			return result, fmt.Errorf("upstream %s/%s failed with HTTP %d", candidate.Provider, candidate.Model, status)
		}
		if err != nil {
			last = err
		} else {
			last = fmt.Errorf("upstream HTTP %d", status)
		}
	}
	if last == nil {
		last = provider.ErrCatalogEmpty
	}
	return result, last
}

func (s *Service) MarkResponseInterrupted(providerID, model string) {
	if s.Health == nil {
		return
	}
	s.Health.Fail(providerID, model, FailureProtocol, time.Now())
}

func (s *Service) ProbeRecovery(ctx context.Context) []RouteHealth {
	if s.Health == nil {
		return nil
	}
	due := s.Health.DueForProbe(time.Now())
	recovered := []RouteHealth{}
	for _, item := range due {
		cfg, ok := s.providers()[item.Provider]
		if !ok {
			continue
		}
		probeModel := item.Model
		if probeModel == "*" {
			probeModel = ""
			for _, model := range s.models() {
				if model.Provider == item.Provider && model.Status == catalog.Active && model.AutoRoutable {
					probeModel = model.ID
					break
				}
			}
			if probeModel == "" {
				continue
			}
		}
		body := []byte(`{"model":"` + probeModel + `","messages":[{"role":"user","content":"Reply OK"}],"max_tokens":2,"stream":false}`)
		pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		resp, err := s.Client.Complete(pctx, cfg.BaseURL, cfg.APIKey, body)
		cancel()
		if err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			s.Health.Success(item.Provider, item.Model, time.Now())
			recovered = append(recovered, item)
		} else {
			s.Health.ProbeFailed(item.Provider, item.Model, time.Now())
		}
		if resp != nil {
			resp.Body.Close()
		}
	}
	return recovered
}
