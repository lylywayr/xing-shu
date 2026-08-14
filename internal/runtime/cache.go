package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type CacheItem struct {
	Body    []byte    `json:"body"`
	Expires time.Time `json:"expires"`
}
type Cache struct {
	mu    sync.Mutex
	items map[string]CacheItem
	TTL   time.Duration
	Max   int
	path  string
}

func NewCache() *Cache    { return &Cache{items: map[string]CacheItem{}, TTL: 5 * time.Minute, Max: 500} }
func Key(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func (c *Cache) Load(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.path = path
	b, e := os.ReadFile(path)
	if e == nil {
		var x map[string]CacheItem
		if json.Unmarshal(b, &x) == nil {
			now := time.Now()
			for k, v := range x {
				if v.Expires.After(now) {
					c.items[k] = v
				}
			}
		}
	}
}
func (c *Cache) persist() {
	if c.path == "" {
		return
	}
	b, _ := json.Marshal(c.items)
	tmp := c.path + ".tmp"
	if os.MkdirAll(filepath.Dir(c.path), 0755) == nil && os.WriteFile(tmp, b, 0644) == nil {
		_ = os.Rename(tmp, c.path)
	}
}
func (c *Cache) Get(k string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	x, ok := c.items[k]
	if !ok || time.Now().After(x.Expires) {
		delete(c.items, k)
		return nil, false
	}
	return append([]byte(nil), x.Body...), true
}
func (c *Cache) Put(k string, b []byte) {
	c.mu.Lock()
	if len(c.items) >= c.Max {
		for k := range c.items {
			delete(c.items, k)
			break
		}
	}
	c.items[k] = CacheItem{Body: append([]byte(nil), b...), Expires: time.Now().Add(c.TTL)}
	c.mu.Unlock()
	c.persist()
}
