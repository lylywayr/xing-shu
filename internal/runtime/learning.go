package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Stat struct {
	Requests  int64     `json:"requests"`
	Success   int64     `json:"success"`
	Failures  int64     `json:"failures"`
	LatencyMS int64     `json:"latency_ms"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Learning struct {
	mu    sync.RWMutex
	items map[string]Stat
	path  string
}

func NewLearning() *Learning { return &Learning{items: map[string]Stat{}} }
func (l *Learning) Load(path string) {
	l.path = path
	b, e := os.ReadFile(path)
	if e == nil {
		var x map[string]Stat
		if json.Unmarshal(b, &x) == nil {
			l.mu.Lock()
			for k, v := range x {
				l.items[k] = v
			}
			l.mu.Unlock()
		}
	}
}
func (l *Learning) persist() {
	if l.path == "" {
		return
	}
	b, _ := json.Marshal(l.Snapshot())
	tmp := l.path + ".tmp"
	if os.MkdirAll(filepath.Dir(l.path), 0755) == nil && os.WriteFile(tmp, b, 0644) == nil {
		_ = os.Rename(tmp, l.path)
	}
}
func (l *Learning) Record(key string, ok bool, lat time.Duration) {
	l.mu.Lock()
	x := l.items[key]
	x.Requests++
	if ok {
		x.Success++
	} else {
		x.Failures++
	}
	x.LatencyMS += lat.Milliseconds()
	x.UpdatedAt = time.Now()
	l.items[key] = x
	l.mu.Unlock()
	l.persist()
}
func (l *Learning) Snapshot() map[string]Stat {
	l.mu.RLock()
	defer l.mu.RUnlock()
	o := map[string]Stat{}
	for k, v := range l.items {
		o[k] = v
	}
	return o
}
