package migration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"xing-shu/internal/catalog"
	"xing-shu/internal/storage"
)

type Report struct {
	Source            string `json:"source"`
	Target            string `json:"target"`
	Models            int    `json:"models"`
	Governance        int    `json:"governance"`
	AllowRules        int    `json:"allow_rules"`
	LearningMigrated  bool   `json:"learning_migrated"`
	AuditMigrated     bool   `json:"audit_migrated"`
	AlertsMigrated    bool   `json:"alerts_migrated"`
	SnapshotsMigrated bool   `json:"snapshots_migrated"`
	FilesMigrated     int    `json:"files_migrated"`
	TargetSHA256      string `json:"target_sha256"`
}

func Run(source, target string) (Report, error) {
	report := Report{Source: source, Target: target}
	if filepath.Clean(source) == filepath.Clean(target) {
		return report, fmt.Errorf("source and target must be different")
	}
	if err := targetAvailable(target); err != nil {
		return report, err
	}
	stage := target + ".migration-tmp"
	if _, err := os.Stat(stage); err == nil {
		return report, fmt.Errorf("migration staging directory exists: %s", stage)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(stage)
		}
	}()
	legacy, err := LoadLegacy(source)
	if err != nil {
		return report, err
	}
	if err := catalog.SaveState(filepath.Join(stage, "catalog-starcore.json"), catalog.State{Catalog: legacy.Catalog, Allow: legacy.Allow}); err != nil {
		return report, err
	}
	if err := storage.SaveGovernance(stage, legacy.Governance); err != nil {
		return report, err
	}
	report.Models, report.Governance, report.AllowRules = len(legacy.Catalog.Models), len(legacy.Governance), len(legacy.Allow)
	for _, item := range []struct {
		name string
		set  func()
	}{
		{"learning.json", func() { report.LearningMigrated = true }},
		{"learning-v2.json", func() { report.LearningMigrated = true }},
		{"audit.jsonl", func() { report.AuditMigrated = true }},
		{"audit-v2.jsonl", func() { report.AuditMigrated = true }},
		{"alerts.jsonl", func() { report.AlertsMigrated = true }},
		{"alerts-state-v2.json", nil},
		{"cache-v2.json", nil},
		{"sessions-v2.json", nil},
		{"reviews-v2.json", nil},
		{"reviewer-session-v2.json", nil},
		{"reviewer-selection-v2.json", nil},
		{"routing-knowledge-v2.json", nil},
		{"shadow-results-v2.json", nil},
		{"shadow-batches-v2.json", nil},
		{"quota-ledger-v2.json", nil},
		{"quota-facts-v2.json", nil},
		{"quota-alert-state-v2.json", nil},
		{"provider-ops-v2.json", nil},
		{"probe-history-v2.json", nil},
	} {
		if item.name == "audit-v2.jsonl" {
			if _, exists := os.Stat(filepath.Join(stage, "audit-starcore.jsonl")); exists == nil {
				continue
			}
		}
		if item.name == "learning-v2.json" {
			if _, exists := os.Stat(filepath.Join(stage, "learning-starcore.json")); exists == nil {
				continue
			}
		}
		if err := copyValidated(source, stage, item.name); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return report, err
		}
		report.FilesMigrated++
		if item.set != nil {
			item.set()
		}
	}
	if info, err := os.Stat(filepath.Join(source, "snapshots-v2")); err == nil && info.IsDir() {
		if err := copyTree(filepath.Join(source, "snapshots-v2"), filepath.Join(stage, "snapshots-starcore")); err != nil {
			return report, err
		}
		report.SnapshotsMigrated = true
	} else if err != nil && !os.IsNotExist(err) {
		return report, err
	}
	if err := os.Rename(stage, target); err != nil {
		return report, err
	}
	report.TargetSHA256, err = directorySHA256(target)
	if err != nil {
		return report, err
	}
	committed = true
	return report, nil
}

func directorySHA256(root string) (string, error) {
	hash := sha256.New()
	var names []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		names = append(names, relative)
		return nil
	}); err != nil {
		return "", err
	}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return "", err
		}
		_, _ = hash.Write([]byte(name + "\\x00"))
		_, _ = hash.Write(data)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0700)
		}
		return copyValidatedFile(path, destination)
	})
}

func copyValidatedFile(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if filepath.Ext(source) == ".json" {
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			return fmt.Errorf("validate %s: %w", source, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}

func targetAvailable(target string) error {
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("target directory already exists: %s", target)
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func copyValidated(source, target, name string) error {
	data, err := os.ReadFile(filepath.Join(source, name))
	if err != nil {
		return err
	}
	if filepath.Ext(name) == ".json" {
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			return fmt.Errorf("validate %s: %w", name, err)
		}
	}
	to := filepath.Join(target, migrationName(name))
	if _, err := os.Stat(to); err == nil {
		return fmt.Errorf("target file exists: %s", to)
	}
	if err := os.MkdirAll(target, 0700); err != nil {
		return err
	}
	tmp := to + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, to)
}

func migrationName(name string) string {
	if name == "learning.json" || name == "learning-v2.json" {
		return "learning-starcore.json"
	}
	if name == "audit.jsonl" || name == "audit-v2.jsonl" {
		return "audit-starcore.jsonl"
	}
	if name == "alerts-state-v2.json" {
		return "alerts-state-starcore.json"
	}
	if len(name) > 7 && name[len(name)-7:] == "-v2.json" {
		return name[:len(name)-7] + "-starcore.json"
	}
	return name
}
