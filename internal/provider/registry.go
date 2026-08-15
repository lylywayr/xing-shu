package provider

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	SourceEnvironment = "environment"
	SourceDynamic     = "dynamic"
	SourceExternal    = "external"
)

var (
	ErrNotFound       = errors.New("provider not found")
	ErrReadOnly       = errors.New("provider is read-only")
	ErrDuplicate      = errors.New("provider already exists")
	ErrKeyUnavailable = errors.New("credential encryption key is unavailable")
	providerIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,63}$`)
)

type ProviderInput struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Kind    string `json:"kind"`
	Enabled *bool  `json:"enabled,omitempty"`
}

type ProviderView struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	BaseURL    string    `json:"base_url"`
	Kind       string    `json:"kind"`
	Source     string    `json:"source"`
	ReadOnly   bool      `json:"read_only"`
	Enabled    bool      `json:"enabled"`
	APIKeyMask string    `json:"api_key_mask"`
	HasAPIKey  bool      `json:"has_api_key"`
	VerifiedAt time.Time `json:"verified_at,omitempty"`
	CreatedAt  time.Time `json:"created_at,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
}

type encryptedSecret struct {
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type dynamicProvider struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	BaseURL    string          `json:"base_url"`
	Kind       string          `json:"kind"`
	Enabled    bool            `json:"enabled"`
	APIKey     encryptedSecret `json:"api_key"`
	APIKeyMask string          `json:"api_key_mask"`
	VerifiedAt time.Time       `json:"verified_at"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

type registryState struct {
	Version   int               `json:"version"`
	Providers []dynamicProvider `json:"providers"`
}

type Registry struct {
	mu        sync.RWMutex
	path      string
	key       []byte
	keyText   string
	env       map[string]Config
	dynamic   map[string]dynamicProvider
	onChanged func()
}

func NewRegistry(path, keyText string, env map[string]Config) (*Registry, error) {
	r := &Registry{path: path, keyText: strings.TrimSpace(keyText), env: cloneConfigs(env), dynamic: map[string]dynamicProvider{}}
	if r.keyText != "" {
		key, err := decodeMasterKey(r.keyText)
		if err != nil {
			return nil, err
		}
		r.key = key
	}
	if err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

func decodeMasterKey(raw string) ([]byte, error) {
	for _, encoding := range []*base64.Encoding{base64.RawStdEncoding, base64.StdEncoding, base64.RawURLEncoding, base64.URLEncoding} {
		if key, err := encoding.DecodeString(raw); err == nil && len(key) == 32 {
			return key, nil
		}
	}
	return nil, errors.New("XING_SHU_CREDENTIAL_KEY must be a base64-encoded 32-byte key")
}

func (r *Registry) masterKeyText() string  { return r.keyText }
func (r *Registry) Ready() bool            { r.mu.RLock(); defer r.mu.RUnlock(); return len(r.key) == 32 }
func (r *Registry) SetOnChanged(fn func()) { r.mu.Lock(); r.onChanged = fn; r.mu.Unlock() }
func (r *Registry) notify() {
	r.mu.RLock()
	fn := r.onChanged
	r.mu.RUnlock()
	if fn != nil {
		fn()
	}
}
func (r *Registry) SetEnvironment(env map[string]Config) {
	r.mu.Lock()
	r.env = cloneConfigs(env)
	r.mu.Unlock()
	r.notify()
}

func NormalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("base_url is required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("base_url must be an absolute http or https URL")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("base_url must not contain credentials, query, or fragment")
	}
	path := strings.TrimRight(u.EscapedPath(), "/")
	for strings.HasSuffix(path, "/v1/v1") {
		path = strings.TrimSuffix(path, "/v1")
	}
	if path == "" {
		path = "/v1"
	}
	if path != "/v1" {
		return "", errors.New("base_url path must be empty or /v1")
	}
	u.Path, u.RawPath = path, ""
	return strings.TrimRight(u.String(), "/"), nil
}

func validateInput(input ProviderInput, requireID bool) (ProviderInput, error) {
	input.ID = strings.ToLower(strings.TrimSpace(input.ID))
	input.Name = strings.TrimSpace(input.Name)
	input.Kind = strings.TrimSpace(input.Kind)
	if requireID && !providerIDPattern.MatchString(input.ID) {
		return input, errors.New("id must be 2-64 lowercase letters, numbers, dot, underscore, or hyphen")
	}
	if input.Name == "" {
		input.Name = input.ID
	}
	if len(input.Name) > 80 {
		return input, errors.New("name is too long")
	}
	base, err := NormalizeBaseURL(input.BaseURL)
	if err != nil {
		return input, err
	}
	input.BaseURL = base
	if input.Kind == "" {
		input.Kind = "standard"
	}
	if input.Kind != "standard" {
		return input, errors.New("only standard OpenAI-compatible providers are supported")
	}
	if len(input.APIKey) > 8192 {
		return input, errors.New("api_key is too long")
	}
	return input, nil
}

func (r *Registry) Create(input ProviderInput) (ProviderView, error) {
	input, err := validateInput(input, true)
	if err != nil {
		return ProviderView{}, err
	}
	if len(r.key) != 32 {
		return ProviderView{}, ErrKeyUnavailable
	}
	r.mu.Lock()
	if _, ok := r.env[input.ID]; ok {
		r.mu.Unlock()
		return ProviderView{}, ErrDuplicate
	}
	if _, ok := ExternalConfigs()[input.ID]; ok {
		r.mu.Unlock()
		return ProviderView{}, ErrDuplicate
	}
	if _, ok := r.dynamic[input.ID]; ok {
		r.mu.Unlock()
		return ProviderView{}, ErrDuplicate
	}
	now := time.Now().UTC()
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	secret, err := r.encrypt(input.APIKey)
	if err != nil {
		r.mu.Unlock()
		return ProviderView{}, err
	}
	record := dynamicProvider{ID: input.ID, Name: input.Name, BaseURL: input.BaseURL, Kind: input.Kind, Enabled: enabled, APIKey: secret, APIKeyMask: MaskAPIKey(input.APIKey), VerifiedAt: now, CreatedAt: now, UpdatedAt: now}
	r.dynamic[input.ID] = record
	if err = r.persistLocked(); err != nil {
		delete(r.dynamic, input.ID)
		r.mu.Unlock()
		return ProviderView{}, err
	}
	r.mu.Unlock()
	r.notify()
	return viewDynamic(record), nil
}

func (r *Registry) ResolveInput(id string, input ProviderInput) (ProviderInput, error) {
	r.mu.RLock()
	record, ok := r.dynamic[id]
	env, envOK := r.env[id]
	r.mu.RUnlock()
	if !ok {
		if !envOK {
			return input, ErrNotFound
		}
		input.ID = id
		if strings.TrimSpace(input.Name) == "" {
			input.Name = first(env.Name, id)
		}
		if strings.TrimSpace(input.BaseURL) == "" {
			input.BaseURL = env.BaseURL
		}
		if strings.TrimSpace(input.Kind) == "" {
			input.Kind = first(env.Kind, "standard")
		}
		if input.Enabled == nil {
			input.Enabled = boolPointer(env.EnabledOrDefault())
		}
		if input.APIKey == "" {
			input.APIKey = env.APIKey
		}
		return validateInput(input, true)
	}
	input.ID = id
	if strings.TrimSpace(input.Name) == "" {
		input.Name = record.Name
	}
	if strings.TrimSpace(input.BaseURL) == "" {
		input.BaseURL = record.BaseURL
	}
	if strings.TrimSpace(input.Kind) == "" {
		input.Kind = record.Kind
	}
	if input.Enabled == nil {
		input.Enabled = boolPointer(record.Enabled)
	}
	if input.APIKey == "" {
		key, err := r.decrypt(record.APIKey)
		if err != nil {
			return input, err
		}
		input.APIKey = key
	}
	return validateInput(input, true)
}

func (r *Registry) Update(id string, input ProviderInput) (ProviderView, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	r.mu.RLock()
	_, env := r.env[id]
	r.mu.RUnlock()
	if env {
		return ProviderView{}, ErrReadOnly
	}
	resolved, err := r.ResolveInput(id, input)
	if err != nil {
		return ProviderView{}, err
	}
	if len(r.key) != 32 {
		return ProviderView{}, ErrKeyUnavailable
	}
	r.mu.Lock()
	old, ok := r.dynamic[id]
	if !ok {
		r.mu.Unlock()
		return ProviderView{}, ErrNotFound
	}
	secret := old.APIKey
	mask := old.APIKeyMask
	if input.APIKey != "" {
		secret, err = r.encrypt(resolved.APIKey)
		if err != nil {
			r.mu.Unlock()
			return ProviderView{}, err
		}
		mask = MaskAPIKey(resolved.APIKey)
	}
	old.Name, old.BaseURL, old.Kind, old.APIKey, old.APIKeyMask, old.VerifiedAt, old.UpdatedAt = resolved.Name, resolved.BaseURL, resolved.Kind, secret, mask, time.Now().UTC(), time.Now().UTC()
	if resolved.Enabled != nil {
		old.Enabled = *resolved.Enabled
	}
	r.dynamic[id] = old
	if err = r.persistLocked(); err != nil {
		r.mu.Unlock()
		return ProviderView{}, err
	}
	r.mu.Unlock()
	r.notify()
	return viewDynamic(old), nil
}

func (r *Registry) SetEnabled(id string, enabled bool) (ProviderView, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	r.mu.Lock()
	if _, ok := r.env[id]; ok {
		r.mu.Unlock()
		return ProviderView{}, ErrReadOnly
	}
	record, ok := r.dynamic[id]
	if !ok {
		r.mu.Unlock()
		return ProviderView{}, ErrNotFound
	}
	record.Enabled, record.UpdatedAt = enabled, time.Now().UTC()
	r.dynamic[id] = record
	if err := r.persistLocked(); err != nil {
		r.mu.Unlock()
		return ProviderView{}, err
	}
	r.mu.Unlock()
	r.notify()
	return viewDynamic(record), nil
}

func (r *Registry) Delete(id string) error {
	id = strings.ToLower(strings.TrimSpace(id))
	r.mu.Lock()
	if _, ok := r.env[id]; ok {
		r.mu.Unlock()
		return ErrReadOnly
	}
	if _, ok := r.dynamic[id]; !ok {
		r.mu.Unlock()
		return ErrNotFound
	}
	delete(r.dynamic, id)
	if err := r.persistLocked(); err != nil {
		r.mu.Unlock()
		return err
	}
	r.mu.Unlock()
	r.notify()
	return nil
}

func (r *Registry) Get(id string) (ProviderView, bool) {
	for _, view := range r.Views() {
		if view.ID == id {
			return view, true
		}
	}
	return ProviderView{}, false
}
func (r *Registry) Views() []ProviderView {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ProviderView, 0, len(r.env)+len(r.dynamic))
	for id, cfg := range r.env {
		out = append(out, ProviderView{ID: id, Name: first(cfg.Name, id), BaseURL: cfg.BaseURL, Kind: first(cfg.Kind, "standard"), Source: SourceEnvironment, ReadOnly: true, Enabled: cfg.EnabledOrDefault(), APIKeyMask: MaskAPIKey(cfg.APIKey), HasAPIKey: cfg.APIKey != ""})
	}
	for _, record := range r.dynamic {
		out = append(out, viewDynamic(record))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func (r *Registry) Snapshot() map[string]Config {
	r.mu.RLock()
	out := cloneConfigs(r.env)
	records := make([]dynamicProvider, 0, len(r.dynamic))
	for _, v := range r.dynamic {
		records = append(records, v)
	}
	r.mu.RUnlock()
	for id, cfg := range out {
		if !cfg.EnabledOrDefault() {
			delete(out, id)
		}
	}
	for _, record := range records {
		if !record.Enabled {
			continue
		}
		key, err := r.decrypt(record.APIKey)
		if err != nil {
			continue
		}
		out[record.ID] = Config{ID: record.ID, Name: record.Name, BaseURL: record.BaseURL, APIKey: key, Kind: record.Kind, Source: SourceDynamic, Enabled: true}
	}
	for id, cfg := range ExternalConfigs() {
		out[id] = cfg
	}
	return out
}
func (r *Registry) Config(id string) (Config, bool) { cfg, ok := r.Snapshot()[id]; return cfg, ok }

func (r *Registry) load() error {
	if r.path == "" {
		return nil
	}
	data, err := os.ReadFile(r.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var state registryState
	if err = json.Unmarshal(data, &state); err != nil {
		return err
	}
	if len(state.Providers) > 0 && len(r.key) != 32 {
		return ErrKeyUnavailable
	}
	for _, record := range state.Providers {
		if _, err = r.decrypt(record.APIKey); err != nil {
			return fmt.Errorf("decrypt provider %s: %w", record.ID, err)
		}
		r.dynamic[record.ID] = record
	}
	return nil
}
func (r *Registry) persistLocked() error {
	if r.path == "" {
		return nil
	}
	records := make([]dynamicProvider, 0, len(r.dynamic))
	for _, v := range r.dynamic {
		records = append(records, v)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	data, err := json.MarshalIndent(registryState{Version: 1, Providers: records}, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(r.path), 0700); err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err = os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, r.path)
}
func (r *Registry) encrypt(plain string) (encryptedSecret, error) {
	block, err := aes.NewCipher(r.key)
	if err != nil {
		return encryptedSecret{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return encryptedSecret{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return encryptedSecret{}, err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plain), nil)
	return encryptedSecret{Nonce: base64.RawStdEncoding.EncodeToString(nonce), Ciphertext: base64.RawStdEncoding.EncodeToString(ciphertext)}, nil
}
func (r *Registry) decrypt(secret encryptedSecret) (string, error) {
	if len(r.key) != 32 {
		return "", ErrKeyUnavailable
	}
	nonce, err := base64.RawStdEncoding.DecodeString(secret.Nonce)
	if err != nil {
		return "", err
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(secret.Ciphertext)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(r.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	return string(plain), err
}
func MaskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	runes := []rune(key)
	if len(runes) <= 8 {
		return strings.Repeat("•", len(runes))
	}
	return string(runes[:4]) + strings.Repeat("•", 6) + string(runes[len(runes)-4:])
}
func viewDynamic(r dynamicProvider) ProviderView {
	return ProviderView{ID: r.ID, Name: r.Name, BaseURL: r.BaseURL, Kind: r.Kind, Source: SourceDynamic, ReadOnly: false, Enabled: r.Enabled, APIKeyMask: r.APIKeyMask, HasAPIKey: r.APIKeyMask != "", VerifiedAt: r.VerifiedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
func cloneConfigs(in map[string]Config) map[string]Config {
	out := map[string]Config{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
func first(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
func boolPointer(v bool) *bool { return &v }
