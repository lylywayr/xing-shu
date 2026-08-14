package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"xing-shu/internal/observability"
	"xing-shu/internal/quota"
	"xing-shu/internal/routing"
	"xing-shu/internal/runtime"
)

type Chat struct {
	Router   *routing.Service
	Fallback http.Handler
	Audit    *observability.Audit
	Ledger   *quota.Ledger
	Runtime  *Runtime
}
type chatFlags struct {
	Model  string `json:"model"`
	Stream bool   `json:"stream"`
	Tools  []any  `json:"tools"`
}

func parseFlags(b []byte) chatFlags { var x chatFlags; _ = json.Unmarshal(b, &x); return x }
func sessionID(r *http.Request) string {
	for _, k := range []string{"X-Conversation-ID", "X-Session-ID", "X-Model-Affinity"} {
		if v := strings.TrimSpace(r.Header.Get(k)); v != "" {
			return v
		}
	}
	return ""
}
func (c *Chat) learn(key string, ok bool, start time.Time) {
	if c.Runtime != nil && c.Runtime.Learning != nil {
		c.Runtime.Learning.Record(key, ok, time.Since(start))
	}
}
func (c *Chat) reviewRecord(flags chatFlags, status int, start time.Time, body, response []byte) {
	if c.Runtime == nil || c.Runtime.Reviews == nil {
		return
	}
	id := time.Now().UTC().Format("20060102T150405.000000000Z07:00") + "-" + runtime.Key([]byte(flags.Model+start.String()))
	features := runtime.TaskFeatures{NeedsTools: len(flags.Tools) > 0, NeedsStructured: strings.Contains(string(body), "response_format"), InputBytes: len(body)}
	providerID := ""
	if c.Router != nil {
		providerID = c.Router.ProviderFor(flags.Model)
	}
	pkg, encrypted := runtime.EncryptTaskPackage(body)
	respPkg, respEncrypted := runtime.EncryptTaskPackage(response)
	c.Runtime.Reviews.Add(runtime.Review{ID: id, RecordedAt: start, Model: flags.Model, Provider: providerID, Status: status, LatencyMS: time.Since(start).Milliseconds(), Stream: flags.Stream, Tools: len(flags.Tools) > 0, Features: features, Replayable: encrypted, TaskPackage: pkg, TaskPackageEncrypted: encrypted, ResponsePackage: respPkg, ResponseCaptured: len(response) > 0, ResponsePackageEncrypted: respEncrypted, ReplayReason: func() string {
		if encrypted {
			return ""
		}
		return "encryption key unavailable"
	}()})
}
func (c *Chat) forward(w http.ResponseWriter, resp *http.Response, flags chatFlags, start time.Time, cacheKey string, requestBody []byte) {
	defer resp.Body.Close()
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	if cacheKey != "" && ok && !flags.Stream {
		b, e := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		if e == nil {
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(b)
			if c.Runtime != nil {
				c.Runtime.Cache.Put(cacheKey, b)
			}
		} else {
			ok = false
			http.Error(w, "upstream read error", 502)
		}
	} else {
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}
	var captured []byte
	if !flags.Stream {
		captured, _ = io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		resp.Body = io.NopCloser(bytes.NewReader(captured))
	}
	c.reviewRecord(flags, resp.StatusCode, start, requestBody, captured)
	c.learn(flags.Model, ok, start)
	if c.Audit != nil {
		c.Audit.Record(observability.Event{Time: time.Now(), Model: flags.Model, Status: resp.StatusCode, LatencyMS: time.Since(start).Milliseconds(), Stream: flags.Stream, Tools: len(flags.Tools) > 0})
	}
}
func (c *Chat) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	body, e := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if e != nil {
		http.Error(w, "read error", 400)
		return
	}
	flags := parseFlags(body)
	sid := sessionID(r)
	if sid != "" && c.Runtime != nil && c.Runtime.Sessions != nil {
		if x, ok := c.Runtime.Sessions.Get(sid); ok && (flags.Model == "auto" || flags.Model == "") && x.Model != "" {
			flags.Model = x.Model
			var q map[string]any
			if json.Unmarshal(body, &q) == nil {
				q["model"] = x.Model
				if b, e := json.Marshal(q); e == nil {
					body = b
				}
			}
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	cacheable := c.Runtime != nil && flags.Model != "auto" && !flags.Stream && len(flags.Tools) == 0
	ck := ""
	if cacheable {
		ck = runtime.Key(body)
		if hit, ok := c.Runtime.Cache.Get(ck); ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(hit)
			c.learn(flags.Model, true, start)
			return
		}
	}
	if flags.Model == "" || flags.Model == "auto" {
		if selected := c.Router.SelectAuto(body); selected != "" {
			flags.Model = selected
			if sid != "" && c.Runtime != nil {
				c.Runtime.Sessions.Put(sid, runtime.Session{Model: selected})
			}
			resp, e := c.Router.Complete(ctx, body, "")
			if e == nil {
				c.forward(w, resp, flags, start, "", body)
				return
			}
		}
		if c.Fallback != nil {
			r.Body = io.NopCloser(bytes.NewReader(body))
			c.Fallback.ServeHTTP(w, r)
			c.learn(flags.Model, false, start)
			return
		}
		http.Error(w, "auto router unavailable", 503)
		c.learn(flags.Model, false, start)
		return
	}
	if !c.Router.HasModel(flags.Model) {
		http.Error(w, "requested model unavailable", 503)
		c.learn(flags.Model, false, start)
		return
	}
	if sid != "" && c.Runtime != nil {
		c.Runtime.Sessions.Put(sid, runtime.Session{Model: strings.TrimPrefix(flags.Model, "openai/")})
	}
	resp, e := c.Router.Complete(ctx, body, "")
	if e != nil {
		if c.Fallback != nil {
			r.Body = io.NopCloser(bytes.NewReader(body))
			c.Fallback.ServeHTTP(w, r)
			c.learn(flags.Model, false, start)
			return
		}
		http.Error(w, e.Error(), 503)
		c.learn(flags.Model, false, start)
		return
	}
	c.forward(w, resp, flags, start, ck, body)
}
func ChatModel(body []byte) string { return parseFlags(body).Model }
func IsStreaming(body []byte) bool {
	return parseFlags(body).Stream || strings.Contains(string(body), "text/event-stream")
}
