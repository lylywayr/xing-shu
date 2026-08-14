package migration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyWritesOnlyStarcoreTarget(t *testing.T) {
	source, target := t.TempDir(), filepath.Join(t.TempDir(), "starcore")
	governance := `{"p/m":{"key":"p/m","provider":"p","model":"m","status":"active"}}`
	if err := os.WriteFile(filepath.Join(source, "governance.json"), []byte(governance), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "learning.json"), []byte(`{"p/m":{"requests":1}}`), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := Run(source, target)
	if err != nil {
		t.Fatal(err)
	}
	if report.Models != 1 || !report.LearningMigrated || report.TargetSHA256 == "" {
		t.Fatalf("unexpected migration report: %+v", report)
	}
	if _, err := os.Stat(filepath.Join(target, "catalog-starcore.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(source, "governance.json")); err != nil {
		t.Fatal("source must remain untouched")
	}
}

func TestMigrateLegacyRejectsExistingTarget(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "governance.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "catalog-starcore.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(source, target); err == nil {
		t.Fatal("existing target must be rejected")
	}
}

func TestRunMigratesSnapshotDirectory(t *testing.T) {
	source, parent := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "governance.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "snapshots-v2"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "snapshots-v2", "one.json"), []byte(`{"models":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "starcore")
	report, err := Run(source, target)
	if err != nil || !report.SnapshotsMigrated {
		t.Fatalf("snapshot migration failed: %+v %v", report, err)
	}
	if _, err := os.Stat(filepath.Join(target, "snapshots-starcore", "one.json")); err != nil {
		t.Fatal(err)
	}
}
