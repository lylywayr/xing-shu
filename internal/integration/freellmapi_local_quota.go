package integration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// LocalQuotaConfig controls the read-only probe of FreeLLMAPI's own SQLite
// data. The database is opened with mode=ro and query_only=ON. Xing Shu never
// migrates, checkpoints, or otherwise writes this database.
type LocalQuotaConfig struct {
	DBPath string
}

type LocalQuotaModel struct {
	Platform          string   `json:"platform"`
	ModelID           string   `json:"model_id"`
	DisplayName       string   `json:"display_name,omitempty"`
	Enabled           bool     `json:"enabled"`
	BudgetLabel       string   `json:"budget_label,omitempty"`
	BudgetTokens      *float64 `json:"budget_tokens"`
	UsedTokens        float64  `json:"used_tokens"`
	RemainingEstimate *float64 `json:"remaining_estimate"`
	Requests          int64    `json:"requests"`
	InputTokens       int64    `json:"input_tokens"`
	OutputTokens      int64    `json:"output_tokens"`
	RPM               *int64   `json:"rpm_limit"`
	RPD               *int64   `json:"rpd_limit"`
	TPM               *int64   `json:"tpm_limit"`
	TPD               *int64   `json:"tpd_limit"`
	RPMUsed           int64    `json:"rpm_used"`
	RPDUsed           int64    `json:"rpd_used"`
	TPMUsed           int64    `json:"tpm_used"`
	TPDUsed           int64    `json:"tpd_used"`
	KeyID             *int64   `json:"key_id"`
	Scope             string   `json:"scope"`
	Source            string   `json:"source"`
}

type LocalQuotaTotals struct {
	BudgetTokens      *float64 `json:"budget_tokens"`
	UsedTokens        float64  `json:"used_tokens"`
	RemainingEstimate *float64 `json:"remaining_estimate"`
	Requests          int64    `json:"requests"`
	InputTokens       int64    `json:"input_tokens"`
	OutputTokens      int64    `json:"output_tokens"`
}

type LocalQuotaReport struct {
	SchemaVersion int               `json:"schema_version"`
	Source        string            `json:"source"`
	DBPath        string            `json:"-"`
	CheckedAt     time.Time         `json:"checked_at"`
	MonthStart    string            `json:"month_start"`
	Totals        LocalQuotaTotals  `json:"totals"`
	Models        []LocalQuotaModel `json:"models"`
	Pools         []QuotaPool       `json:"quota_pools"`
	Partial       bool              `json:"partial"`
	Warnings      []string          `json:"warnings,omitempty"`
}

type localUsage struct {
	Requests, InputTokens, OutputTokens int64
}
type localRateUsage struct {
	RPM, RPD, TPM, TPD int64
}
type localModelRow struct {
	Platform, ModelID, DisplayName, BudgetLabel string
	RPM, RPD, TPM, TPD, KeyID                   sql.NullInt64
	ModelEnabled, RouteEnabled                  int64
	Usage                                       localUsage
}

// ReadLocalQuota reads the FreeLLMAPI database only after the integration has
// been authorized. It is deliberately a Xing Shu-side adapter: FreeLLMAPI is
// not changed and no admin session, encrypted key, prompt, or response is read.
func (m *Manager) ReadLocalQuota(ctx context.Context, cfg LocalQuotaConfig) (*LocalQuotaReport, error) {
	if !m.Authorized() {
		return nil, errors.New("freellmapi integration not authorized")
	}
	path := filepath.Clean(strings.TrimSpace(cfg.DBPath))
	if path == "." || path == "" || !filepath.IsAbs(path) {
		return nil, errors.New("freellmapi local database is not configured")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	dsn := "file:" + path + "?mode=ro"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, errors.New("freellmapi local database unavailable")
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, errors.New("freellmapi local database unavailable")
	}
	if _, err := db.ExecContext(ctx, "PRAGMA query_only = ON"); err != nil {
		return nil, errors.New("freellmapi local database is not read-only")
	}
	var queryOnly int
	if err := db.QueryRowContext(ctx, "PRAGMA query_only").Scan(&queryOnly); err != nil || queryOnly != 1 {
		return nil, errors.New("freellmapi local database is not read-only")
	}

	report := &LocalQuotaReport{
		SchemaVersion: 1,
		Source:        "freellmapi_sqlite_readonly",
		DBPath:        path,
		CheckedAt:     time.Now().UTC(),
	}
	if err := db.QueryRowContext(ctx, "SELECT strftime('%Y-%m-01 00:00:00','now')").Scan(&report.MonthStart); err != nil {
		return nil, errors.New("freellmapi local database query failed")
	}
	if err := readLocalModels(ctx, db, report); err != nil {
		return nil, err
	}
	if err := readLocalPools(ctx, db, &report.Pools); err != nil {
		return nil, err
	}
	if len(report.Models) == 0 {
		report.Partial = true
		report.Warnings = append(report.Warnings, "no_enabled_fallback_models")
	}
	return report, nil
}

func readLocalModels(ctx context.Context, db *sql.DB, report *LocalQuotaReport) error {
	platforms, err := queryStringSet(ctx, db, "SELECT DISTINCT platform FROM api_keys WHERE enabled = 1")
	if err != nil {
		return errors.New("freellmapi enabled providers unavailable")
	}
	keyCounts := map[string]int{}
	keyRows, err := db.QueryContext(ctx, "SELECT platform, COUNT(*) FROM api_keys WHERE enabled = 1 AND status IN ('healthy','unknown') GROUP BY platform")
	if err != nil {
		return errors.New("freellmapi provider key counts unavailable")
	}
	for keyRows.Next() {
		var platform string
		var count int
		if err := keyRows.Scan(&platform, &count); err != nil {
			keyRows.Close()
			return errors.New("freellmapi provider key counts unavailable")
		}
		keyCounts[platform] = count
	}
	if err := keyRows.Err(); err != nil {
		keyRows.Close()
		return errors.New("freellmapi provider key counts unavailable")
	}
	keyRows.Close()

	usage := map[string]localUsage{}
	usageRows, err := db.QueryContext(ctx, `SELECT platform, model_id, COUNT(*), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0) FROM requests WHERE created_at >= datetime('now','start of month') AND request_type = 'chat' GROUP BY platform, model_id`)
	if err != nil {
		return errors.New("freellmapi model usage unavailable")
	}
	for usageRows.Next() {
		var platform, model string
		var u localUsage
		if err := usageRows.Scan(&platform, &model, &u.Requests, &u.InputTokens, &u.OutputTokens); err != nil {
			usageRows.Close()
			return errors.New("freellmapi model usage unavailable")
		}
		usage[platform+"\x00"+model] = u
	}
	if err := usageRows.Err(); err != nil {
		usageRows.Close()
		return errors.New("freellmapi model usage unavailable")
	}
	usageRows.Close()

	rate, err := readLocalRateUsage(ctx, db)
	if err != nil {
		return err
	}

	activeProfile := ""
	_ = db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'active_profile_id'").Scan(&activeProfile)
	var rows *sql.Rows
	profileID, profileErr := strconv.ParseInt(strings.TrimSpace(activeProfile), 10, 64)
	profileExists := false
	if profileErr == nil && profileID > 0 {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM profiles WHERE id = ?", profileID).Scan(&count); err == nil {
			profileExists = count > 0
		}
	}
	if profileExists {
		rows, err = db.QueryContext(ctx, `SELECT m.platform,m.model_id,COALESCE(m.display_name,''),m.monthly_token_budget,m.rpm_limit,m.rpd_limit,m.tpm_limit,m.tpd_limit,m.enabled,m.key_id,pm.enabled FROM profile_models pm JOIN models m ON m.id = pm.model_db_id WHERE pm.profile_id = ? AND m.enabled = 1 ORDER BY pm.priority ASC`, profileID)
	} else {
		rows, err = db.QueryContext(ctx, `SELECT m.platform,m.model_id,COALESCE(m.display_name,''),m.monthly_token_budget,m.rpm_limit,m.rpd_limit,m.tpm_limit,m.tpd_limit,m.enabled,m.key_id,fc.enabled FROM fallback_config fc JOIN models m ON m.id = fc.model_db_id WHERE m.enabled = 1 ORDER BY fc.priority ASC`)
	}
	if err != nil {
		return errors.New("freellmapi fallback model catalog unavailable")
	}
	defer rows.Close()
	for rows.Next() {
		var row localModelRow
		var budget sql.NullString
		if err := rows.Scan(&row.Platform, &row.ModelID, &row.DisplayName, &budget, &row.RPM, &row.RPD, &row.TPM, &row.TPD, &row.ModelEnabled, &row.KeyID, &row.RouteEnabled); err != nil {
			return errors.New("freellmapi fallback model catalog unavailable")
		}
		// The final value is the fallback/profile enabled flag. FreeLLMAPI's
		// token-usage page preserves the row, but the monthly usage query remains
		// the authoritative source for aggregate usage.
		if !platforms[row.Platform] {
			continue
		}
		row.BudgetLabel = budget.String
		row.Usage = usage[row.Platform+"\x00"+row.ModelID]
		budgetTokens := parseBudgetLabel(row.BudgetLabel)
		if budgetTokens != nil {
			keys := keyCounts[row.Platform]
			if keys < 1 {
				keys = 1
			}
			*budgetTokens *= float64(keys)
		} else {
			report.Partial = true
		}
		model := LocalQuotaModel{
			Platform: row.Platform, ModelID: row.ModelID, DisplayName: row.DisplayName,
			Enabled: row.RouteEnabled != 0, BudgetLabel: row.BudgetLabel, BudgetTokens: budgetTokens,
			UsedTokens: float64(row.Usage.InputTokens + row.Usage.OutputTokens),
			Requests:   row.Usage.Requests, InputTokens: row.Usage.InputTokens, OutputTokens: row.Usage.OutputTokens,
			RPM: nullableInt(row.RPM), RPD: nullableInt(row.RPD), TPM: nullableInt(row.TPM), TPD: nullableInt(row.TPD),
			KeyID: nullableInt(row.KeyID), Scope: "model", Source: "freellmapi_sqlite",
		}
		model.RemainingEstimate = remaining(model.BudgetTokens, model.UsedTokens)
		if r := rate[row.Platform+"\x00"+row.ModelID]; r != nil {
			model.RPMUsed, model.RPDUsed, model.TPMUsed, model.TPDUsed = r.RPM, r.RPD, r.TPM, r.TPD
		}
		report.Models = append(report.Models, model)
		report.Totals.Requests += model.Requests
		report.Totals.InputTokens += model.InputTokens
		report.Totals.OutputTokens += model.OutputTokens
		report.Totals.UsedTokens += model.UsedTokens
		if model.BudgetTokens != nil {
			if report.Totals.BudgetTokens == nil {
				v := 0.0
				report.Totals.BudgetTokens = &v
			}
			*report.Totals.BudgetTokens += *model.BudgetTokens
		}
	}
	if err := rows.Err(); err != nil {
		return errors.New("freellmapi fallback model catalog unavailable")
	}
	report.Totals.RemainingEstimate = remaining(report.Totals.BudgetTokens, report.Totals.UsedTokens)
	return nil
}

func readLocalRateUsage(ctx context.Context, db *sql.DB) (map[string]*localRateUsage, error) {
	now := time.Now()
	minute := now.Add(-time.Minute).UnixMilli()
	day := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC).UnixMilli()
	rows, err := db.QueryContext(ctx, `SELECT platform,model_id, SUM(CASE WHEN kind='request' AND created_at_ms > ? THEN 1 ELSE 0 END), SUM(CASE WHEN kind='request' AND created_at_ms > ? THEN 1 ELSE 0 END), SUM(CASE WHEN kind='tokens' AND created_at_ms > ? THEN tokens ELSE 0 END), SUM(CASE WHEN kind='tokens' AND created_at_ms > ? THEN tokens ELSE 0 END) FROM rate_limit_usage WHERE created_at_ms > ? GROUP BY platform,model_id`, minute, day, minute, day, day)
	if err != nil {
		return nil, errors.New("freellmapi rate usage unavailable")
	}
	defer rows.Close()
	out := map[string]*localRateUsage{}
	for rows.Next() {
		var platform, model string
		var r localRateUsage
		if err := rows.Scan(&platform, &model, &r.RPM, &r.RPD, &r.TPM, &r.TPD); err != nil {
			return nil, errors.New("freellmapi rate usage unavailable")
		}
		out[platform+"\x00"+model] = &r
	}
	return out, rows.Err()
}

func readLocalPools(ctx context.Context, db *sql.DB, pools *[]QuotaPool) error {
	rows, err := db.QueryContext(ctx, `WITH latest AS (SELECT platform,key_id,quota_pool_key,metric,model_id,observed_at,ROW_NUMBER() OVER (PARTITION BY platform,key_id,quota_pool_key,metric ORDER BY observed_at DESC,created_at DESC) rn FROM provider_quota_observations) SELECT p.platform,p.key_id,p.quota_pool_key,p.metric,p.limit_value,p.remaining_value,p.reset_at,p.reset_strategy,p.source,p.confidence,p.notes,COALESCE(l.model_id,''),COALESCE(l.observed_at,'') FROM provider_quota_state p LEFT JOIN latest l ON l.platform=p.platform AND l.key_id=p.key_id AND l.quota_pool_key=p.quota_pool_key AND l.metric=p.metric AND l.rn=1 ORDER BY p.platform,p.key_id,p.quota_pool_key,p.metric`)
	if err != nil {
		return errors.New("freellmapi quota pools unavailable")
	}
	defer rows.Close()
	for rows.Next() {
		var platform, pool, metric, reset, strategy, source, notes, modelID, observedAt sql.NullString
		var key, limit, remain sql.NullInt64
		var confidence sql.NullFloat64
		if err := rows.Scan(&platform, &key, &pool, &metric, &limit, &remain, &reset, &strategy, &source, &confidence, &notes, &modelID, &observedAt); err != nil {
			return errors.New("freellmapi quota pools unavailable")
		}
		id := pool.String
		if key.Valid {
			id = fmt.Sprintf("%s#%d", id, key.Int64)
		}
		q := QuotaPool{ID: id, Platform: platform.String, KeyID: nullableInt(key), ModelID: modelID.String, Metric: metric.String, Unit: metric.String, ResetAt: reset.String, ResetStrategy: strategy.String, Source: source.String, Confidence: confidence.Float64, Notes: notes.String, ObservedAt: observedAt.String, Partial: !limit.Valid || !remain.Valid}
		q.Limit, q.Remaining = nullableFloat(limit), nullableFloat(remain)
		if q.Limit != nil && q.Remaining != nil {
			v := math.Max(0, *q.Limit-*q.Remaining)
			q.Used = &v
		}
		*pools = append(*pools, q)
	}
	return rows.Err()
}

func queryStringSet(ctx context.Context, db *sql.DB, query string) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		out[value] = true
	}
	return out, rows.Err()
}

var budgetPattern = regexp.MustCompile(`~?([0-9]+(?:\.[0-9]+)?)(?:-([0-9]+(?:\.[0-9]+)?))?([MK])`)

func parseBudgetLabel(s string) *float64 {
	match := budgetPattern.FindStringSubmatch(strings.ToUpper(s))
	if len(match) != 4 {
		return nil
	}
	value := match[1]
	if match[2] != "" {
		value = match[2]
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || n < 0 {
		return nil
	}
	if match[3] == "M" {
		n *= 1_000_000
	} else {
		n *= 1_000
	}
	return &n
}

func remaining(budget *float64, used float64) *float64 {
	if budget == nil {
		return nil
	}
	value := math.Max(0, *budget-used)
	return &value
}

func nullableInt(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	copy := value.Int64
	return &copy
}

func nullableFloat(value sql.NullInt64) *float64 {
	if !value.Valid {
		return nil
	}
	copy := float64(value.Int64)
	return &copy
}
