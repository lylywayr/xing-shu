package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"xing-shu/internal/clientkey"
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

type flushWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func (f flushWriter) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	f.f.Flush()
	return n, err
}

func (c *Chat) forward(w http.ResponseWriter, resp *http.Response, flags chatFlags, start time.Time, cacheKey string, requestBody []byte) bool {
	defer resp.Body.Close()
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	var captured []byte
	if !flags.Stream {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		if err != nil {
			ok = false
			http.Error(w, "upstream read error", http.StatusBadGateway)
		} else {
			captured = body
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(body)
			if cacheKey != "" && ok && c.Runtime != nil {
				c.Runtime.Cache.Put(cacheKey, body)
			}
		}
	} else {
		w.WriteHeader(resp.StatusCode)
		var target io.Writer = w
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
			target = flushWriter{w: w, f: flusher}
		}
		_, err := io.Copy(target, resp.Body)
		if err != nil {
			ok = false
		}
	}
	c.reviewRecord(flags, resp.StatusCode, start, requestBody, captured)
	if c.Audit != nil {
		errorText := ""
		if !ok {
			errorText = "upstream_response_interrupted"
		}
		c.Audit.Record(observability.Event{Time: time.Now(), Action: "route.response", Model: flags.Model, Status: resp.StatusCode, LatencyMS: time.Since(start).Milliseconds(), Stream: flags.Stream, Tools: len(flags.Tools) > 0, Error: errorText})
	}
	return ok
}
func (c *Chat) recordRoute(start time.Time, flags chatFlags, result *routing.ReliableResult, clientKey string, finalError error) {
	if c.Audit == nil {
		return
	}
	if result == nil {
		errorText := "route unavailable"
		if finalError != nil {
			errorText = finalError.Error()
		}
		c.Audit.Record(observability.Event{Time: time.Now().UTC(), Action: "route.complete", Model: flags.Model, Status: http.StatusServiceUnavailable, LatencyMS: time.Since(start).Milliseconds(), Stream: flags.Stream, Tools: len(flags.Tools) > 0, Error: errorText, ClientKey: clientKey})
		return
	}
	status := 0
	if result.Response != nil {
		status = result.Response.StatusCode
	}
	errorText := ""
	if finalError != nil {
		errorText = finalError.Error()
	}
	switches := 0
	if len(result.Attempts) > 1 {
		switches = len(result.Attempts) - 1
	}
	ttfb := int64(0)
	if len(result.Attempts) > 0 {
		ttfb = result.Attempts[len(result.Attempts)-1].TTFBMS
	}
	c.Audit.Record(observability.Event{Time: time.Now().UTC(), Action: "route.complete", Model: result.Model, Provider: result.Provider, Status: status, LatencyMS: time.Since(start).Milliseconds(), TTFBMS: ttfb, Switches: switches, Stream: flags.Stream, Tools: len(flags.Tools) > 0, Error: errorText, ClientKey: clientKey, Attempts: result.Attempts})
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
	clientKeyPrefix := "migration-optional"
	if key, ok := clientkey.FromContext(r.Context()); ok {
		clientKeyPrefix = key.Prefix
	}
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
	if flags.Model != "" && flags.Model != "auto" && !c.Router.HasModel(flags.Model) {
		http.Error(w, "requested model unavailable", 503)
		c.learn(flags.Model, false, start)
		return
	}
	result, routeErr := c.Router.CompleteReliable(ctx, body, routing.ReliableOptions{MaxAttempts: 3, PerAttemptTimeout: 45 * time.Second})
	if routeErr != nil || result == nil || result.Response == nil {
		c.recordRoute(start, flags, result, clientKeyPrefix, routeErr)
		if c.Fallback != nil {
			r.Body = io.NopCloser(bytes.NewReader(body))
			c.Fallback.ServeHTTP(w, r)
			c.learn(flags.Model, false, start)
			return
		}
		http.Error(w, "all approved routes unavailable", http.StatusServiceUnavailable)
		c.learn(flags.Model, false, start)
		return
	}
	flags.Model = result.Model
	if sid != "" && c.Runtime != nil {
		c.Runtime.Sessions.Put(sid, runtime.Session{Model: result.Model})
	}
	ok := c.forward(w, result.Response, flags, start, ck, body)
	if !ok {
		c.Router.MarkResponseInterrupted(result.Provider, result.Model)
		c.recordRoute(start, flags, result, clientKeyPrefix, errors.New("upstream response interrupted after output began"))
		return
	}
	c.recordRoute(start, flags, result, clientKeyPrefix, nil)
}
func ChatModel(body []byte) string { return parseFlags(body).Model }
func IsStreaming(body []byte) bool {
	return parseFlags(body).Stream || strings.Contains(string(body), "text/event-stream")
}
