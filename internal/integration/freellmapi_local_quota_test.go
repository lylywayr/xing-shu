package integration

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func makeLocalQuotaDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "freeapi.db")
	db, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE api_keys (id INTEGER PRIMARY KEY, platform TEXT, status TEXT, enabled INTEGER)`,
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT)`,
		`CREATE TABLE models (id INTEGER PRIMARY KEY, platform TEXT, model_id TEXT, display_name TEXT, monthly_token_budget TEXT, rpm_limit INTEGER, rpd_limit INTEGER, tpm_limit INTEGER, tpd_limit INTEGER, enabled INTEGER, key_id INTEGER)`,
		`CREATE TABLE fallback_config (model_db_id INTEGER, priority INTEGER, enabled INTEGER)`,
		`CREATE TABLE profile_models (profile_id INTEGER, model_db_id INTEGER, priority INTEGER, enabled INTEGER)`,
		`CREATE TABLE requests (platform TEXT, model_id TEXT, status TEXT, input_tokens INTEGER, output_tokens INTEGER, created_at TEXT, request_type TEXT)`,
		`CREATE TABLE rate_limit_usage (platform TEXT, model_id TEXT, key_id INTEGER, kind TEXT, tokens INTEGER, created_at_ms INTEGER)`,
		`CREATE TABLE provider_quota_observations (platform TEXT, key_id INTEGER, quota_pool_key TEXT, metric TEXT, model_id TEXT, observed_at TEXT, created_at TEXT)`,
		`CREATE TABLE provider_quota_state (platform TEXT, key_id INTEGER, quota_pool_key TEXT, metric TEXT, limit_value INTEGER, remaining_value INTEGER, reset_at TEXT, reset_strategy TEXT, source TEXT, confidence REAL, notes TEXT)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	_, err = db.Exec(`INSERT INTO api_keys VALUES (1,'google','healthy',1); INSERT INTO models VALUES (1,'google','gemini-test','Gemini test','~1-2M',10,20,1000,5000,1,1); INSERT INTO fallback_config VALUES (1,1,1); INSERT INTO requests VALUES ('google','gemini-test','success',120,30,datetime('now'),'chat'); INSERT INTO provider_quota_state VALUES ('google',1,'google::project','requests',100,80,NULL,'unknown','test',0.8,'')`)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadLocalQuotaRequiresAuthorization(t *testing.T) {
	m, err := New(filepath.Join(t.TempDir(), "state.json"), "http://127.0.0.1:3001", "key", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.ReadLocalQuota(context.Background(), LocalQuotaConfig{DBPath: "/does/not/exist"}); err == nil {
		t.Fatal("unauthorized read must fail")
	}
}

func TestReadLocalQuotaIsReadOnlyAndAggregatesModelsAndPools(t *testing.T) {
	path := makeLocalQuotaDB(t)
	m, err := New(filepath.Join(t.TempDir(), "state.json"), "http://127.0.0.1:3001", "key", false)
	if err != nil {
		t.Fatal(err)
	}
	m.Authorize(false)
	report, err := m.ReadLocalQuota(context.Background(), LocalQuotaConfig{DBPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Models) != 1 || report.Models[0].UsedTokens != 150 || report.Models[0].BudgetTokens == nil || *report.Models[0].BudgetTokens != 2_000_000 {
		t.Fatalf("unexpected model report: %+v", report.Models)
	}
	if report.Totals.Requests != 1 || report.Totals.UsedTokens != 150 || len(report.Pools) != 1 || report.Pools[0].Remaining == nil || *report.Pools[0].Remaining != 80 {
		t.Fatalf("unexpected totals/pools: %+v", report)
	}
	if report.Models[0].RPMUsed != 0 {
		t.Fatalf("unexpected rate usage: %+v", report.Models[0])
	}
	probe, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	if _, err := probe.Exec(`INSERT INTO models VALUES (2,'x','should-not-write','x','~1M',NULL,NULL,NULL,NULL,1,NULL)`); err == nil {
		t.Fatal("read-only database must reject writes")
	}
}

func TestBudgetLabelsUseUpperBoundAndUnknownIsPartial(t *testing.T) {
	for input, expected := range map[string]float64{"~1-2M": 2_000_000, "~500K": 500_000} {
		got := parseBudgetLabel(input)
		if got == nil || *got != expected {
			t.Fatalf("%s -> %v, want %v", input, got, expected)
		}
	}
	if got := parseBudgetLabel("free · promo"); got != nil {
		t.Fatalf("unknown label parsed: %v", *got)
	}
}

func TestLocalQuotaSnapshotSurvivesReadFailure(t *testing.T) {
	path := makeLocalQuotaDB(t)
	m, err := New(filepath.Join(t.TempDir(), "state.json"), "http://127.0.0.1:3001", "key", false)
	if err != nil {
		t.Fatal(err)
	}
	m.SetLocalQuotaPath(path)
	m.Authorize(false)
	if _, err := m.RefreshLocalQuota(context.Background()); err != nil {
		t.Fatal(err)
	}
	before, _, _ := m.LocalQuotaState()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RefreshLocalQuota(context.Background()); err == nil {
		t.Fatal("missing database must fail")
	}
	after, _, errText := m.LocalQuotaState()
	if before == nil || after == nil || errText == "" {
		t.Fatalf("last-known-good snapshot not retained: before=%v after=%v error=%q", before, after, errText)
	}
}
