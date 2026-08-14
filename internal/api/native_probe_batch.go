package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"xing-shu/internal/catalog"
)

type ProbeTarget struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type ProbeBatchRequest struct {
	Models []ProbeTarget `json:"models"`
}

type ProbeBatchHistory struct {
	JobID     string    `json:"job_id"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	Total     int       `json:"total"`
	Completed int       `json:"completed"`
	Failed    int       `json:"failed"`
	Status    string    `json:"status"`
}

type probeBatchJob struct {
	ProbeBatchHistory
	Results []map[string]any `json:"results"`
}

type ProbeBatchRuntime struct {
	probe   *ProbeRuntime
	dir     string
	mu      sync.RWMutex
	jobs    map[string]*probeBatchJob
	history []ProbeBatchHistory
	counter uint64
}

func NewProbeBatchRuntime(probe *ProbeRuntime, dir string) *ProbeBatchRuntime {
	b := &ProbeBatchRuntime{probe: probe, dir: dir, jobs: map[string]*probeBatchJob{}}
	b.load()
	return b
}

func (b *ProbeBatchRuntime) Start(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input ProbeBatchRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&input); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	targets, err := b.validate(input.Models)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	b.mu.Lock()
	b.counter++
	id := fmt.Sprintf("probe-%d-%d", time.Now().UnixNano(), b.counter)
	now := time.Now().UTC()
	b.jobs[id] = &probeBatchJob{ProbeBatchHistory: ProbeBatchHistory{JobID: id, StartedAt: now, Total: len(targets), Status: "running"}}
	b.mu.Unlock()
	go b.run(id, targets)
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]any{"job_id": id, "status": "running", "total": len(targets)})
}

func (b *ProbeBatchRuntime) validate(targets []ProbeTarget) ([]ProbeTarget, error) {
	if len(targets) == 0 || len(targets) > 50 {
		return nil, errors.New("models must contain 1 to 50 targets")
	}
	seen := map[string]bool{}
	valid := make([]ProbeTarget, 0, len(targets))
	for _, target := range targets {
		key := target.Provider + "\x00" + target.Model
		if target.Provider == "" || target.Model == "" || seen[key] {
			return nil, errors.New("models contain an empty or duplicate target")
		}
		seen[key] = true
		if b.probe == nil || b.probe.Manager == nil {
			return nil, errors.New("probe runtime unavailable")
		}
		found := false
		for _, model := range b.probe.Manager.Snapshot().Models {
			if model.Provider == target.Provider && model.ID == target.Model && model.Status == catalog.Active {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("model not active: %s/%s", target.Provider, target.Model)
		}
		if _, ok := b.probe.Configs[target.Provider]; !ok {
			return nil, fmt.Errorf("provider not found: %s", target.Provider)
		}
		valid = append(valid, target)
	}
	return valid, nil
}

func (b *ProbeBatchRuntime) run(id string, targets []ProbeTarget) {
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, target := range targets {
		wg.Add(1)
		go func(target ProbeTarget) {
			defer wg.Done()
			sem <- struct{}{}
			result := b.probeOne(target)
			<-sem
			b.mu.Lock()
			job := b.jobs[id]
			job.Completed++
			if result["ok"] != true {
				job.Failed++
			}
			job.Results = append(job.Results, result)
			b.mu.Unlock()
		}(target)
	}
	wg.Wait()
	b.mu.Lock()
	job := b.jobs[id]
	job.Status = "completed"
	job.EndedAt = time.Now().UTC()
	b.history = append([]ProbeBatchHistory(nil), append(b.history, job.ProbeBatchHistory)...)
	if len(b.history) > 100 {
		b.history = b.history[len(b.history)-100:]
	}
	b.saveLocked()
	b.mu.Unlock()
}

func (b *ProbeBatchRuntime) probeOne(target ProbeTarget) map[string]any {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	ok, status, err := b.probe.probeModel(ctx, target.Provider, target.Model)
	result := map[string]any{"provider": target.Provider, "model": target.Model, "ok": ok, "status": status, "checked_at": time.Now().UTC().Format(time.RFC3339)}
	if err != nil {
		result["error"] = err.Error()
	}
	return result
}

func (b *ProbeBatchRuntime) Get(id string) map[string]any {
	b.mu.RLock()
	defer b.mu.RUnlock()
	job, ok := b.jobs[id]
	if !ok {
		return map[string]any{"status": "not_found"}
	}
	return map[string]any{"job_id": job.JobID, "status": job.Status, "total": float64(job.Total), "completed": float64(job.Completed), "failed": float64(job.Failed), "results": job.Results}
}

func (b *ProbeBatchRuntime) Status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("job_id"))
	if id == "" {
		http.Error(w, "job_id is required", http.StatusBadRequest)
		return
	}
	result := b.Get(id)
	if result["status"] == "not_found" {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	writeJSON(w, result)
}

func (b *ProbeBatchRuntime) HistoryItems() []ProbeBatchHistory {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return append([]ProbeBatchHistory(nil), b.history...)
}

func (b *ProbeBatchRuntime) History(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b.mu.RLock()
	items := make([]ProbeBatchHistory, len(b.history))
	copy(items, b.history)
	b.mu.RUnlock()
	writeJSON(w, map[string]any{"items": items})
}

func (b *ProbeBatchRuntime) load() {
	if b.dir == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(b.dir, "probe-history-starcore.json"))
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &b.history)
}

func (b *ProbeBatchRuntime) saveLocked() {
	if b.dir == "" {
		return
	}
	_ = os.MkdirAll(b.dir, 0700)
	data, err := json.MarshalIndent(b.history, "", "  ")
	if err != nil {
		return
	}
	tmp := filepath.Join(b.dir, "probe-history-starcore.json.tmp")
	if os.WriteFile(tmp, data, 0600) == nil {
		_ = os.Rename(tmp, filepath.Join(b.dir, "probe-history-starcore.json"))
	}
}
