package migration

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
	"xing-shu/internal/catalog"
	"xing-shu/internal/governance"
	"xing-shu/internal/quota"
)

type Legacy struct {
	Catalog    catalog.Catalog
	Governance []governance.Record
	Quota      []quota.Snapshot
	Allow      map[string]bool
	Profiles   map[string]Profile
}
type Profile struct {
	Capabilities  []string `json:"capabilities"`
	ContextWindow int      `json:"context_window"`
	Tools         bool     `json:"tools"`
	Vision        bool     `json:"vision"`
	Source        string   `json:"source"`
	Confidence    float64  `json:"confidence"`
}

func LoadLegacy(dir string) (Legacy, error) {
	var out Legacy
	out.Allow = map[string]bool{}
	out.Profiles = map[string]Profile{}
	b, e := os.ReadFile(filepath.Join(dir, "governance.json"))
	if e != nil {
		return out, e
	}
	var raw map[string]struct {
		Key          string         `json:"key"`
		Provider     string         `json:"provider"`
		Model        string         `json:"model"`
		Status       catalog.Status `json:"status"`
		Reason       string         `json:"reason"`
		Source       string         `json:"source"`
		SourceLevel  string         `json:"source_level"`
		EvidenceURL  string         `json:"evidence_url"`
		EvidenceHash string         `json:"evidence_hash"`
		CheckedAt    time.Time      `json:"checked_at"`
		LastSeen     time.Time      `json:"last_seen"`
		Missing      int            `json:"missing_syncs"`
	}
	if e = json.Unmarshal(b, &raw); e != nil {
		return out, e
	}
	if p, e := os.ReadFile(filepath.Join(dir, "model_profiles.json")); e == nil {
		_ = json.Unmarshal(p, &out.Profiles)
	}
	for k, v := range raw {
		m := catalog.Model{ID: v.Model, Provider: v.Provider, Status: v.Status, AutoRoutable: v.Status == catalog.Active, Object: "model", OwnedBy: v.Provider, UpdatedAt: v.CheckedAt}
		if m.ID == "" {
			parts := strings.SplitN(k, "/", 2)
			if len(parts) == 2 {
				m.Provider, m.ID = parts[0], parts[1]
			}
		}
		if p, ok := out.Profiles[k]; ok {
			m.Capabilities = p.Capabilities
			m.ContextWindow = p.ContextWindow
			m.Tools = p.Tools
			m.Vision = p.Vision
		}
		r := governance.Record{Key: k, Model: m, Status: v.Status, Reason: v.Reason, MissingSyncs: v.Missing, CheckedAt: v.CheckedAt, LastSeen: v.LastSeen}
		r.Evidence = []catalog.Evidence{{Source: v.Source, Level: v.SourceLevel, URL: v.EvidenceURL, Hash: v.EvidenceHash, Supported: true, Confidence: .55, CheckedAt: v.CheckedAt}}
		out.Catalog.Models = append(out.Catalog.Models, m)
		out.Governance = append(out.Governance, r)
	}
	out.Catalog.SyncedAt = time.Now()
	out.Catalog.Version = "migrated-v1"
	if f, e := os.Open(filepath.Join(dir, "auto_allow.txt")); e == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			out.Allow[strings.TrimSpace(sc.Text())] = true
		}
	}
	for i := range out.Catalog.Models {
		key := out.Catalog.Models[i].Provider + "/" + out.Catalog.Models[i].ID
		out.Catalog.Models[i].AutoRoutable = out.Allow[key] && out.Catalog.Models[i].Status == catalog.Active
	}
	for i := range out.Governance {
		out.Governance[i].Model.AutoRoutable = out.Allow[out.Governance[i].Key] && out.Governance[i].Status == catalog.Active
	}
	return out, nil
}
