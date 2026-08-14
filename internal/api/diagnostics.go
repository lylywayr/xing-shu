package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type RuntimeFileMetric struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Exists    bool      `json:"exists"`
	Bytes     int64     `json:"bytes"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	Records   int       `json:"records,omitempty"`
	Error     string    `json:"error,omitempty"`
}

func RuntimeDiagnostics(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if dataDir == "" {
			dataDir = "/data"
		}
		names := []string{"catalog-starcore.json", "governance-starcore.json", "cache-starcore.json", "sessions-starcore.json", "learning-starcore.json", "reviews-starcore.json", "routing-knowledge-starcore.json", "shadow-results-starcore.json", "shadow-batches-starcore.json", "quota-ledger-starcore.json", "audit-starcore.jsonl", "alerts.jsonl", "quota-alert-state-starcore.json", "provider-ops-starcore.json"}
		metrics := make([]RuntimeFileMetric, 0, len(names))
		for _, name := range names {
			metrics = append(metrics, fileMetric(dataDir, name))
		}
		writeJSON(w, map[string]any{"data_dir": dataDir, "items": metrics, "generated_at": time.Now().UTC()})
	}
}
func fileMetric(dir, name string) RuntimeFileMetric {
	path := filepath.Join(dir, name)
	metric := RuntimeFileMetric{Name: name, Path: path}
	info, err := os.Stat(path)
	if err != nil {
		metric.Error = "not_found"
		return metric
	}
	metric.Exists, metric.Bytes, metric.UpdatedAt = true, info.Size(), info.ModTime().UTC()
	if strings.HasSuffix(name, ".jsonl") {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			metric.Records = len(strings.Split(strings.TrimSpace(string(b)), "\n"))
		}
	}
	return metric
}

type WatchdogStatus struct {
	StateDir            string `json:"state_dir"`
	Healthy             bool   `json:"healthy"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
	MaxFailures         int    `json:"max_failures"`
	LastFailureAt       string `json:"last_failure_at,omitempty"`
	DryRun              bool   `json:"dry_run"`
	Detail              string `json:"detail"`
}

func WatchdogStatusView(stateDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if stateDir == "" {
			stateDir = os.Getenv("STARCORE_WATCHDOG_STATE_DIR")
			if stateDir == "" {
				stateDir = "/var/run/starcore-watchdog"
			}
		}
		maxFailures := 5
		if value, err := strconv.Atoi(os.Getenv("STARCORE_WATCHDOG_MAX_FAILURES")); err == nil && value > 0 {
			maxFailures = value
		}
		failures := readInt(filepath.Join(stateDir, "failures"))
		lastFailure := readText(filepath.Join(stateDir, "last_failure_at"))
		status := WatchdogStatus{StateDir: stateDir, Healthy: failures == 0, ConsecutiveFailures: failures, MaxFailures: maxFailures, LastFailureAt: lastFailure, DryRun: os.Getenv("STARCORE_WATCHDOG_DRY_RUN") == "1", Detail: "星枢看门狗尚未报告故障"}
		if failures > 0 {
			status.Detail = fmt.Sprintf("星枢看门狗连续失败 %d 次", failures)
		}
		writeJSON(w, status)
	}
}
func readInt(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}
func readText(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
func (s WatchdogStatus) MarshalJSON() ([]byte, error) {
	type alias WatchdogStatus
	return json.Marshal(alias(s))
}
