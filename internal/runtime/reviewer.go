package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ReviewDecision struct {
	ID               string  `json:"id"`
	RecommendedModel string  `json:"recommended_model"`
	MinimumTier      int     `json:"minimum_tier"`
	Overqualified    bool    `json:"overqualified"`
	Confidence       float64 `json:"confidence"`
	Conclusion       string  `json:"conclusion"`
	Skip             bool    `json:"skip"`
}
type ReviewModel struct {
	ID               string   `json:"id"`
	Provider         string   `json:"provider"`
	Status           string   `json:"status"`
	AutoRoutable     bool     `json:"auto_routable"`
	Capabilities     []string `json:"capabilities,omitempty"`
	Tools            bool     `json:"tools"`
	Vision           bool     `json:"vision"`
	StructuredOutput bool     `json:"structured_output"`
	ContextWindow    int      `json:"context_window"`
}
type Reviewer struct {
	Reviews         *Reviews
	Session         *ReviewSession
	AvailableModels []ReviewModel
	BaseURL         string
	APIKey          string
	Model           string
	Client          *http.Client
	MaxChars        int
	CompactEvery    int
}

func (r *Reviewer) Run(ctx context.Context, before time.Time, limit int) (map[string]any, error) {
	if r.Reviews == nil {
		return nil, fmt.Errorf("reviews unavailable")
	}
	if r.BaseURL == "" || r.Model == "" {
		return nil, fmt.Errorf("reviewer not configured")
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	batch := r.Reviews.Claim(before, time.Now().UTC().Format("20060102T150405Z"), limit)
	if len(batch) == 0 {
		return map[string]any{"claimed": 0, "reviewed": 0}, nil
	}
	if r.Session == nil {
		r.Session = NewReviewSession()
	}
	if r.MaxChars <= 0 {
		r.MaxChars = 48000
	}
	if r.CompactEvery <= 0 {
		r.CompactEvery = 20
	}
	msgs, summary, _, _ := r.Session.Snapshot()
	if len(msgs) > 0 && len(msgs)%r.CompactEvery == 0 {
		_ = r.compact(ctx, msgs, summary)
	}
	msgs, summary, _, _ = r.Session.Snapshot()
	prompt := recordsJSON(batch)
	messages := r.messages(summary, msgs, prompt)
	payload := map[string]any{"model": r.Model, "messages": messages, "response_format": map[string]string{"type": "json_object"}}
	raw, e := r.call(ctx, payload)
	if e != nil && isContextError(e) {
		if ce := r.compact(ctx, msgs, summary); ce == nil {
			msgs, summary, _, _ = r.Session.Snapshot()
			payload["messages"] = r.messages(summary, msgs, prompt)
			raw, e = r.call(ctx, payload)
		}
	}
	if e != nil {
		r.markFailed(batch, e.Error())
		return nil, e
	}
	var out struct {
		Reviews []ReviewDecision `json:"reviews"`
	}
	if json.Unmarshal([]byte(raw), &out) != nil {
		e = fmt.Errorf("reviewer output is not strict json")
		r.markFailed(batch, e.Error())
		return nil, e
	}
	valid := map[string]bool{}
	for _, x := range batch {
		valid[x.ID] = true
	}
	available := map[string]bool{}
	for _, m := range r.AvailableModels {
		available[m.ID] = true
	}
	count := 0
	for _, d := range out.Reviews {
		if !valid[d.ID] || d.Confidence < 0 || d.Confidence > 1 || d.MinimumTier < 0 || d.MinimumTier > 10 {
			continue
		}
		if d.RecommendedModel != "" && !available[d.RecommendedModel] {
			continue
		}
		now := time.Now()
		r.Reviews.Apply(d.ID, func(x *Review) {
			x.ReviewedAt = &now
			x.ReviewStatus = StatusReviewed
			x.ReviewerModel = r.Model
			x.RecommendedModel = d.RecommendedModel
			x.MinimumTier = d.MinimumTier
			x.Overqualified = d.Overqualified
			x.Confidence = d.Confidence
			x.Conclusion = d.Conclusion
			x.ReviewError = ""
			if d.Skip {
				x.ReviewStatus = StatusSkipped
			}
		})
		count++
	}
	for _, x := range batch {
		found := false
		for _, d := range out.Reviews {
			if d.ID == x.ID {
				found = true
				break
			}
		}
		if !found {
			r.Reviews.Apply(x.ID, func(v *Review) { v.ReviewStatus = StatusFailed; v.ReviewError = "missing or invalid decision" })
		}
	}
	r.Session.Append(prompt, string(mustJSON(out)))
	if r.Session.ApproxChars() > r.MaxChars {
		_ = r.compact(ctx, nil, "")
	}
	return map[string]any{"claimed": len(batch), "reviewed": count, "available_models": len(r.AvailableModels)}, nil
}
func (r *Reviewer) messages(summary string, msgs []ReviewMessage, prompt string) []any {
	system := "You are a routing review teacher. Maintain one continuous review conversation. Base every recommendation ONLY on the CURRENT AVAILABLE MODELS list below. Never invent a model. A model recommendation is valid only if its exact ID is in that list and it is active, auto-routable, and has the required capabilities. Return ONLY strict JSON with a reviews array.\nCURRENT AVAILABLE MODELS:\n" + modelsJSON(r.AvailableModels)
	m := []any{map[string]string{"role": "system", "content": system}}
	if summary != "" {
		m = append(m, map[string]string{"role": "system", "content": "Long-term review memory:\n" + summary})
	}
	for _, x := range msgs {
		m = append(m, x)
	}
	return append(m, map[string]string{"role": "user", "content": "Review this new batch and return one decision per ID:\n" + prompt})
}
func (r *Reviewer) call(ctx context.Context, payload map[string]any) (string, error) {
	b, _ := json.Marshal(payload)
	req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(r.BaseURL, "/")+"/chat/completions", bytes.NewReader(b))
	if e != nil {
		return "", e
	}
	req.Header.Set("Content-Type", "application/json")
	if r.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+r.APIKey)
	}
	c := r.Client
	if c == nil {
		c = http.DefaultClient
	}
	res, e := c.Do(req)
	if e != nil {
		return "", e
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("reviewer http %d", res.StatusCode)
	}
	var env struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &env) != nil || len(env.Choices) == 0 {
		return "", fmt.Errorf("invalid reviewer response")
	}
	return env.Choices[0].Message.Content, nil
}
func (r *Reviewer) compact(ctx context.Context, msgs []ReviewMessage, summary string) error {
	if len(msgs) == 0 {
		return nil
	}
	p := map[string]any{"model": r.Model, "messages": []any{map[string]string{"role": "system", "content": "Compress this routing review history into concise durable memory. Keep task patterns, model sufficiency findings, contradictions and confidence. Return plain text only."}, map[string]string{"role": "user", "content": summary + "\n" + messagesText(msgs)}}}
	raw, e := r.call(ctx, p)
	if e != nil {
		return e
	}
	r.Session.Replace(raw, nil)
	return nil
}
func isContextError(e error) bool {
	if e == nil {
		return false
	}
	s := strings.ToLower(e.Error())
	return strings.Contains(s, "context") || strings.Contains(s, "token") || strings.Contains(s, "length") || strings.Contains(s, "413")
}
func messagesText(xs []ReviewMessage) string {
	var b strings.Builder
	for _, x := range xs {
		b.WriteString(x.Role + ":" + x.Content + "\n")
	}
	return b.String()
}
func (r *Reviewer) markFailed(batch []Review, msg string) {
	for _, x := range batch {
		r.Reviews.Apply(x.ID, func(v *Review) { v.ReviewStatus = StatusFailed; v.ReviewError = msg })
	}
}
func recordsJSON(xs []Review) string {
	type item struct {
		ID        string       `json:"id"`
		Model     string       `json:"model"`
		Provider  string       `json:"provider"`
		Status    int          `json:"status"`
		LatencyMS int64        `json:"latency_ms"`
		Stream    bool         `json:"stream"`
		Tools     bool         `json:"tools"`
		Features  TaskFeatures `json:"features"`
	}
	a := make([]item, 0, len(xs))
	for _, x := range xs {
		a = append(a, item{x.ID, x.Model, x.Provider, x.Status, x.LatencyMS, x.Stream, x.Tools, x.Features})
	}
	b, _ := json.Marshal(a)
	return string(b)
}
func modelsJSON(xs []ReviewModel) string { b, _ := json.Marshal(xs); return string(b) }
func mustJSON(v any) []byte              { b, _ := json.Marshal(v); return b }
