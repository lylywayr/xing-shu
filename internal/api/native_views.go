package api

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"xing-shu/internal/runtime"
)

func ReadJSONL(path string) []map[string]any {
	f, e := os.Open(path)
	if e != nil {
		return []map[string]any{}
	}
	defer f.Close()
	out := []map[string]any{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var x map[string]any
		if json.Unmarshal(sc.Bytes(), &x) == nil {
			out = append(out, x)
		}
	}
	if len(out) > 100 {
		out = out[len(out)-100:]
	}
	return out
}
func LearningView(v any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if x, ok := v.(*runtime.Learning); ok {
			writeJSON(w, map[string]any{"items": x.Snapshot()})
			return
		}
		writeJSON(w, map[string]any{"items": []any{}})
	}
}
func AuditView(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items := filterAudit(ReadJSONL(path), r)
		limit, offset := queryLimit(r, 50, 200), queryOffset(r)
		if offset > len(items) {
			offset = len(items)
		}
		end := offset + limit
		if end > len(items) {
			end = len(items)
		}
		writeJSON(w, map[string]any{"requests": items[offset:end], "total": len(items), "limit": limit, "offset": offset})
	}
}

func filterAudit(items []map[string]any, r *http.Request) []map[string]any {
	model, provider, status := strings.TrimSpace(r.URL.Query().Get("model")), strings.TrimSpace(r.URL.Query().Get("provider")), strings.TrimSpace(r.URL.Query().Get("status"))
	minLatency, _ := strconv.ParseInt(r.URL.Query().Get("min_latency_ms"), 10, 64)
	maxLatency, _ := strconv.ParseInt(r.URL.Query().Get("max_latency_ms"), 10, 64)
	tools, stream := r.URL.Query().Get("tools"), r.URL.Query().Get("stream")
	from, to := parseTimeQuery(r.URL.Query().Get("from")), parseTimeQuery(r.URL.Query().Get("to"))
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if model != "" && !strings.Contains(strings.ToLower(fmt.Sprint(item["model"])), strings.ToLower(model)) {
			continue
		}
		if provider != "" && fmt.Sprint(item["provider"]) != provider {
			continue
		}
		if status != "" && fmt.Sprint(item["status"]) != status {
			continue
		}
		latency := int64Value(item["latency_ms"])
		if minLatency > 0 && latency < minLatency || maxLatency > 0 && latency > maxLatency {
			continue
		}
		if tools != "" && boolText(item["tools"]) != tools {
			continue
		}
		if stream != "" && boolText(item["stream"]) != stream {
			continue
		}
		at := auditTime(item)
		if !from.IsZero() && (at.IsZero() || at.Before(from)) || !to.IsZero() && (at.IsZero() || at.After(to)) {
			continue
		}
		out = append(out, item)
	}
	return out
}
func int64Value(value any) int64 {
	switch x := value.(type) {
	case float64:
		return int64(x)
	case int:
		return int64(x)
	case int64:
		return x
	default:
		n, _ := strconv.ParseInt(fmt.Sprint(value), 10, 64)
		return n
	}
}
func boolText(value any) string {
	if value == nil {
		return ""
	}
	if x, ok := value.(bool); ok {
		if x {
			return "true"
		}
		return "false"
	}
	return strings.ToLower(fmt.Sprint(value))
}
func parseTimeQuery(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	x, _ := time.Parse(time.RFC3339, value)
	return x
}
func auditTime(item map[string]any) time.Time {
	for _, key := range []string{"time", "recorded_at", "created_at", "at"} {
		if value, ok := item[key].(string); ok {
			if x, e := time.Parse(time.RFC3339, value); e == nil {
				return x
			}
		}
	}
	return time.Time{}
}

type AlertActionRecord struct {
	Action string    `json:"action"`
	Status string    `json:"status"`
	Note   string    `json:"note,omitempty"`
	At     time.Time `json:"at"`
}
type AlertState struct {
	Status    string              `json:"status"`
	Note      string              `json:"note,omitempty"`
	UpdatedAt time.Time           `json:"updated_at"`
	History   []AlertActionRecord `json:"history,omitempty"`
}

func alertStatePath(path string) string {
	return filepath.Join(filepath.Dir(path), "alerts-state-xing-shu.json")
}
func loadAlertStates(path string) map[string]AlertState {
	out := map[string]AlertState{}
	b, e := os.ReadFile(alertStatePath(path))
	if e == nil {
		_ = json.Unmarshal(b, &out)
	}
	return out
}
func saveAlertStates(path string, states map[string]AlertState) error {
	b, e := json.Marshal(states)
	if e != nil {
		return e
	}
	p := alertStatePath(path)
	if e = os.MkdirAll(filepath.Dir(p), 0755); e != nil {
		return e
	}
	tmp := p + ".tmp"
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	return os.Rename(tmp, p)
}
func alertItems(path string) []map[string]any {
	items := ReadJSONL(path)
	states := loadAlertStates(path)
	for i, item := range items {
		id := fmt.Sprint(item["id"])
		if id == "" || id == "<nil>" {
			sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%v|%v|%v", i, item["level"], item["message"], item["at"])))
			id = "alert-" + hex.EncodeToString(sum[:8])
		}
		item["id"] = id
		if state, ok := states[id]; ok {
			item["handling_status"] = state.Status
			item["handling_note"] = state.Note
			item["handling_updated_at"] = state.UpdatedAt
			item["handling_history"] = state.History
		} else if item["handling_status"] == nil {
			item["handling_status"] = "open"
		}
		items[i] = item
	}
	return items
}
func AlertsView(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items := alertItems(path)
		status, level := r.URL.Query().Get("status"), r.URL.Query().Get("level")
		filtered := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if status != "" && fmt.Sprint(item["handling_status"]) != status {
				continue
			}
			if level != "" && fmt.Sprint(item["level"]) != level {
				continue
			}
			filtered = append(filtered, item)
		}
		limit, offset := queryLimit(r, 50, 200), queryOffset(r)
		if offset > len(filtered) {
			offset = len(filtered)
		}
		end := offset + limit
		if end > len(filtered) {
			end = len(filtered)
		}
		writeJSON(w, map[string]any{"items": filtered[offset:end], "total": len(filtered), "limit": limit, "offset": offset})
	}
}
func AlertAction(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var q struct {
			ID     string `json:"id"`
			Action string `json:"action"`
			Note   string `json:"note"`
		}
		if json.NewDecoder(r.Body).Decode(&q) != nil || strings.TrimSpace(q.ID) == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		allowed := map[string]string{"acknowledge": "acknowledged", "ignore": "ignored", "resolve": "resolved", "reopen": "open"}
		status, ok := allowed[q.Action]
		if !ok {
			http.Error(w, "invalid alert action", http.StatusBadRequest)
			return
		}
		found := false
		for _, item := range alertItems(path) {
			if fmt.Sprint(item["id"]) == q.ID {
				found = true
				break
			}
		}
		if !found {
			http.Error(w, "alert not found", http.StatusNotFound)
			return
		}
		states := loadAlertStates(path)
		state := states[q.ID]
		now := time.Now().UTC()
		state.Status, state.Note, state.UpdatedAt = status, q.Note, now
		state.History = append(state.History, AlertActionRecord{Action: q.Action, Status: status, Note: q.Note, At: now})
		states[q.ID] = state
		if err := saveAlertStates(path, states); err != nil {
			http.Error(w, "alert state unavailable", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "id": q.ID, "status": status, "note": q.Note})
	}
}
