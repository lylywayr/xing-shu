package observability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Event struct {
	Time      time.Time `json:"time"`
	Action    string    `json:"action,omitempty"`
	Model     string    `json:"model"`
	Provider  string    `json:"provider"`
	Status    int       `json:"status"`
	LatencyMS int64     `json:"latency_ms"`
	Stream    bool      `json:"stream"`
	Tools     bool      `json:"tools"`
	Error     string    `json:"error,omitempty"`
}
type Audit struct {
	Dir string
	mu  sync.Mutex
}

func New(dir string) *Audit { _ = os.MkdirAll(dir, 0700); return &Audit{Dir: dir} }
func (a *Audit) Record(e Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	f, x := os.OpenFile(filepath.Join(a.Dir, "audit-starcore.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if x != nil {
		return
	}
	defer f.Close()
	b, _ := json.Marshal(e)
	_, _ = f.Write(append(b, '\n'))
}
