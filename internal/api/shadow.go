package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"xing-shu/internal/catalog"
	"xing-shu/internal/provider"
	"xing-shu/internal/runtime"
)

func ShadowView(s *runtime.ShadowStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s == nil {
			writeJSON(w, map[string]any{"items": []any{}, "total": 0, "limit": queryLimit(r, 50, 200), "offset": queryOffset(r)})
			return
		}
		status, batchID, knowledgeID := strings.TrimSpace(r.URL.Query().Get("status")), strings.TrimSpace(r.URL.Query().Get("batch_id")), strings.TrimSpace(r.URL.Query().Get("knowledge_id"))
		all := s.All()
		filtered := make([]runtime.ShadowResult, 0, len(all))
		for _, item := range all {
			if (status == "" || string(item.Status) == status) && (batchID == "" || item.BatchID == batchID) && (knowledgeID == "" || item.KnowledgeID == knowledgeID) {
				filtered = append(filtered, item)
			}
		}
		limit, offset := queryLimit(r, 50, 200), queryOffset(r)
		if offset > len(filtered) {
			offset = len(filtered)
		}
		end := offset + limit
		if end > len(filtered) {
			end = len(filtered)
		}
		writeJSON(w, map[string]any{"items": filtered[offset:end], "total": len(filtered), "limit": limit, "offset": offset, "status": status, "batch_id": batchID, "knowledge_id": knowledgeID})
	}
}
func ShadowRun(rt *Runtime, manager *catalog.Manager, ops *OpsState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || rt == nil || rt.Shadow == nil || rt.Knowledge == nil || rt.Reviews == nil {
			http.Error(w, "shadow unavailable", 503)
			return
		}
		var q struct {
			KnowledgeID    string `json:"knowledge_id"`
			ReviewID       string `json:"review_id"`
			CandidateModel string `json:"candidate_model"`
			BatchID        string `json:"batch_id"`
		}
		if json.NewDecoder(r.Body).Decode(&q) != nil || q.KnowledgeID == "" || q.ReviewID == "" || strings.TrimSpace(q.CandidateModel) == "" {
			http.Error(w, "knowledge_id, review_id and candidate_model required", http.StatusBadRequest)
			return
		}
		var k runtime.Knowledge
		for _, x := range rt.Knowledge.All() {
			if x.ID == q.KnowledgeID {
				k = x
				break
			}
		}
		rev, ok := rt.Reviews.Find(q.ReviewID)
		if k.ID == "" || !ok {
			http.Error(w, "source not found", 404)
			return
		}
		if k.Status != runtime.KnowledgeTrusted {
			http.Error(w, "knowledge must be trusted", http.StatusConflict)
			return
		}
		if !rev.Replayable || len(rev.TaskPackage) == 0 {
			http.Error(w, "task package unavailable", 409)
			return
		}
		candidate := q.CandidateModel
		if candidate == "" {
			candidate = k.RecommendedModel
		}
		if !modelAvailable(manager, ops, candidate) {
			http.Error(w, "candidate unavailable", http.StatusConflict)
			return
		}
		var cm catalog.Model
		for _, m := range manager.Snapshot().Models {
			if m.ID == candidate {
				cm = m
				break
			}
		}
		pc, ok := provider.LoadConfigs()[cm.Provider]
		if !ok || pc.BaseURL == "" {
			http.Error(w, "provider unavailable", 503)
			return
		}
		id := q.KnowledgeID + "|" + q.ReviewID + "|" + candidate
		if q.BatchID != "" {
			id = q.BatchID + "|" + id
		}
		now := time.Now()
		rt.Shadow.Put(runtime.ShadowResult{ID: id, KnowledgeID: q.KnowledgeID, ReviewID: q.ReviewID, BatchID: q.BatchID, CandidateModel: candidate, Status: runtime.ShadowRunning, CreatedAt: now, UpdatedAt: now})
		plain, ok := runtime.DecryptTaskPackage(rev.TaskPackage)
		if !ok {
			rt.Shadow.Apply(id, func(x *runtime.ShadowResult) {
				x.Status = runtime.ShadowFailed
				x.Error = "task package unavailable"
				x.Cause = runtime.CauseProtocol
			})
			http.Error(w, "task package unavailable", http.StatusConflict)
			return
		}
		body := replaceShadowModel(runtime.SanitizeShadowRequest(plain), candidate)
		ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
		defer cancel()
		start := time.Now()
		req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(pc.BaseURL, "/")+"/chat/completions", strings.NewReader(string(body)))
		if e != nil {
			shadowFail(rt, id, e)
			http.Error(w, e.Error(), 502)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if pc.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+pc.APIKey)
		}
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			shadowFail(rt, id, e)
			http.Error(w, e.Error(), 502)
			return
		}
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 32<<20))
		res.Body.Close()
		success := res.StatusCode >= 200 && res.StatusCode < 300
		original := []byte(nil)
		if rev.ResponseCaptured {
			original = rev.ResponsePackage
		}
		quality := runtime.EvaluateResponse(raw, body, original)
		protocol, nonEmpty, structured := quality.Protocol, quality.Content, quality.Structured
		success = success && protocol && nonEmpty && quality.ToolCompatible && (!parseShadowNeedsStructured(body) || structured)
		cause := runtime.CauseUnknown
		if res.StatusCode == 429 {
			cause = runtime.CauseRateLimit
		} else if res.StatusCode >= 500 {
			cause = runtime.CauseProvider
		} else if !protocol || !quality.ToolCompatible {
			cause = runtime.CauseProtocol
		}
		rt.Shadow.Apply(id, func(x *runtime.ShadowResult) {
			x.Status = runtime.ShadowCompleted
			x.HTTPStatus = res.StatusCode
			x.LatencyMS = time.Since(start).Milliseconds()
			x.ProtocolOK = protocol
			x.StructuredOK = structured
			x.ToolCompatible = quality.ToolCompatible
			x.SemanticMatch = quality.SemanticMatch
			x.QualityConclusion = quality.Conclusion
			x.Success = success
			x.Cause = cause
		})
		_ = rt.Knowledge.RecordValidation(k.ID, "shadow-"+id, success && protocol, cause, "automatic shadow replay")
		writeJSON(w, map[string]any{"ok": true, "id": id, "success": success && protocol, "status": res.StatusCode, "protocol_ok": protocol})
	}
}
func shadowResponseQuality(raw []byte, request []byte) (bool, bool, bool) {
	if !json.Valid(raw) {
		return false, false, false
	}
	var x struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			}
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &x) != nil || len(x.Choices) == 0 {
		return true, false, false
	}
	content := x.Choices[0].Message.Content
	non := content != nil && strings.TrimSpace(fmt.Sprint(content)) != ""
	return true, non, non
}
func parseShadowNeedsStructured(b []byte) bool {
	var x struct {
		ResponseFormat any `json:"response_format"`
	}
	_ = json.Unmarshal(b, &x)
	return x.ResponseFormat != nil
}
func shadowFail(rt *Runtime, id string, e error) {
	rt.Shadow.Apply(id, func(x *runtime.ShadowResult) {
		x.Status = runtime.ShadowFailed
		x.Error = e.Error()
		x.Cause = runtime.CauseProvider
	})
}
func replaceShadowModel(b []byte, name string) []byte {
	var x map[string]any
	if json.Unmarshal(b, &x) != nil {
		return b
	}
	x["model"] = name
	y, _ := json.Marshal(x)
	return y
}
