package clientkey

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrUnauthorized = errors.New("invalid client API key")
var ErrForbidden = errors.New("client API key scope denied")

type Key struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	Enabled    bool       `json:"enabled"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	Hash       string     `json:"hash,omitempty"`
	Secret     string     `json:"-"`
}
type CreateInput struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}
type diskState struct {
	Version int   `json:"version"`
	Mode    Mode  `json:"mode"`
	Keys    []Key `json:"keys"`
}
type Store struct {
	mu   sync.RWMutex
	path string
	keys map[string]Key
	mode Mode
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, keys: map[string]Key{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var state diskState
	if err = json.Unmarshal(b, &state); err != nil {
		return nil, err
	}
	s.mode = state.Mode
	if s.mode == "" {
		s.mode = ModeOptional
	}
	for _, k := range state.Keys {
		s.keys[k.ID] = k
	}
	return s, nil
}
func normalizeScopes(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if (v == "models:read" || v == "chat:write") && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func hashSecret(v string) string { x := sha256.Sum256([]byte(v)); return hex.EncodeToString(x[:]) }
func sanitized(k Key) Key {
	k.Hash = ""
	k.Secret = ""
	k.Scopes = append([]string(nil), k.Scopes...)
	return k
}
func (s *Store) Mode() Mode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.mode == "" {
		return ModeOptional
	}
	return s.mode
}
func (s *Store) SetMode(mode Mode) error {
	if mode != ModeOptional && mode != ModeRequired {
		return errors.New("invalid authentication mode")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if mode == ModeRequired {
		active := false
		now := time.Now()
		for _, key := range s.keys {
			if key.Enabled && key.RevokedAt == nil && (key.ExpiresAt == nil || key.ExpiresAt.After(now)) {
				active = true
				break
			}
		}
		if !active {
			return errors.New("create an active client API key before requiring authentication")
		}
	}
	s.mode = mode
	return s.persistLocked()
}
func (s *Store) Create(in CreateInput) (Key, string, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Key{}, "", errors.New("name is required")
	}
	scopes := normalizeScopes(in.Scopes)
	if len(scopes) == 0 {
		return Key{}, "", errors.New("at least one valid scope is required")
	}
	id, e := randomToken(9)
	if e != nil {
		return Key{}, "", e
	}
	raw, e := randomToken(32)
	if e != nil {
		return Key{}, "", e
	}
	secret := "xsk_" + raw
	now := time.Now().UTC()
	prefix := secret
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	k := Key{ID: id, Name: name, Prefix: prefix, Scopes: scopes, Enabled: true, CreatedAt: now, ExpiresAt: in.ExpiresAt, Hash: hashSecret(secret)}
	s.mu.Lock()
	s.keys[id] = k
	e = s.persistLocked()
	s.mu.Unlock()
	if e != nil {
		return Key{}, "", e
	}
	return sanitized(k), secret, nil
}
func (s *Store) List() []Key {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Key, 0, len(s.keys))
	for _, k := range s.keys {
		out = append(out, sanitized(k))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}
func (s *Store) Revoke(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.keys[id]
	if !ok {
		return os.ErrNotExist
	}
	now := time.Now().UTC()
	k.Enabled = false
	k.RevokedAt = &now
	s.keys[id] = k
	return s.persistLocked()
}
func (s *Store) SetEnabled(id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.keys[id]
	if !ok {
		return os.ErrNotExist
	}
	if k.RevokedAt != nil && enabled {
		return errors.New("revoked key cannot be enabled")
	}
	k.Enabled = enabled
	s.keys[id] = k
	return s.persistLocked()
}
func (s *Store) Rotate(id string) (Key, string, error) {
	rawID, err := randomToken(9)
	if err != nil {
		return Key{}, "", err
	}
	rawSecret, err := randomToken(32)
	if err != nil {
		return Key{}, "", err
	}
	secret := "xsk_" + rawSecret
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.keys[id]
	if !ok {
		return Key{}, "", os.ErrNotExist
	}
	original := old
	now := time.Now().UTC()
	old.Enabled = false
	old.RevokedAt = &now
	s.keys[id] = old
	prefix := secret
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	next := Key{ID: rawID, Name: old.Name + " (rotated)", Prefix: prefix, Scopes: append([]string(nil), old.Scopes...), Enabled: true, CreatedAt: now, ExpiresAt: old.ExpiresAt, Hash: hashSecret(secret)}
	s.keys[next.ID] = next
	if err := s.persistLocked(); err != nil {
		delete(s.keys, next.ID)
		s.keys[id] = original
		return Key{}, "", err
	}
	return sanitized(next), secret, nil
}
func (s *Store) Authenticate(r *http.Request, scope string, now time.Time) (Key, error) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return Key{}, ErrUnauthorized
	}
	secret := strings.TrimSpace(header[7:])
	candidateBytes := sha256.Sum256([]byte(secret))
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, k := range s.keys {
		hashBytes, err := hex.DecodeString(k.Hash)
		if err != nil || len(hashBytes) != sha256.Size || subtle.ConstantTimeCompare(candidateBytes[:], hashBytes) != 1 {
			continue
		}
		if !k.Enabled || k.RevokedAt != nil || (k.ExpiresAt != nil && !k.ExpiresAt.After(now)) {
			return Key{}, ErrUnauthorized
		}
		allowed := false
		for _, v := range k.Scopes {
			if v == scope {
				allowed = true
				break
			}
		}
		if !allowed {
			return Key{}, ErrForbidden
		}
		used := now.UTC()
		persistUsage := k.LastUsedAt == nil || used.Sub(*k.LastUsedAt) >= time.Minute
		k.LastUsedAt = &used
		s.keys[id] = k
		if persistUsage {
			_ = s.persistLocked()
		}
		return sanitized(k), nil
	}
	return Key{}, ErrUnauthorized
}
func (s *Store) persistLocked() error {
	if e := os.MkdirAll(filepath.Dir(s.path), 0700); e != nil {
		return e
	}
	items := make([]Key, 0, len(s.keys))
	for _, k := range s.keys {
		items = append(items, k)
	}
	b, e := json.MarshalIndent(diskState{Version: 1, Mode: s.mode, Keys: items}, "", "  ")
	if e != nil {
		return e
	}
	tmp := s.path + ".tmp"
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	return os.Rename(tmp, s.path)
}
