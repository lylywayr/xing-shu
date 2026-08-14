package quota

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Reservation struct {
	Provider string    `json:"provider"`
	Tokens   int       `json:"tokens"`
	At       time.Time `json:"at"`
}
type Ledger struct {
	mu       sync.Mutex
	Reserved map[string]int
	path     string
}

func NewLedger() *Ledger { return &Ledger{Reserved: map[string]int{}} }
func (l *Ledger) Load(path string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.path = path
	b, e := os.ReadFile(path)
	if e == nil {
		_ = json.Unmarshal(b, &l.Reserved)
	}
	if l.Reserved == nil {
		l.Reserved = map[string]int{}
	}
}
func (l *Ledger) persist() {
	if l.path == "" {
		return
	}
	b, _ := json.Marshal(l.Reserved)
	tmp := l.path + ".tmp"
	if os.MkdirAll(filepath.Dir(l.path), 0755) == nil && os.WriteFile(tmp, b, 0644) == nil {
		_ = os.Rename(tmp, l.path)
	}
}
func (l *Ledger) Reserve(provider string, tokens int) Reservation {
	l.mu.Lock()
	l.Reserved[provider] += tokens
	r := Reservation{Provider: provider, Tokens: tokens, At: time.Now()}
	l.mu.Unlock()
	l.persist()
	return r
}
func (l *Ledger) Release(r Reservation) {
	l.mu.Lock()
	l.Reserved[r.Provider] -= r.Tokens
	if l.Reserved[r.Provider] < 0 {
		l.Reserved[r.Provider] = 0
	}
	l.mu.Unlock()
	l.persist()
}
func (l *Ledger) Snapshot() map[string]int {
	l.mu.Lock()
	defer l.mu.Unlock()
	o := map[string]int{}
	for k, v := range l.Reserved {
		o[k] = v
	}
	return o
}
