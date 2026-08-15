package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"xing-shu/internal/auth"
	"xing-shu/internal/catalog"
	"xing-shu/internal/clientkey"
	"xing-shu/internal/governance"
	"xing-shu/internal/provider"
	"xing-shu/internal/quota"
	"xing-shu/internal/routing"
	"xing-shu/internal/runtime"
)

type Server struct {
	Auth               auth.Authorizer
	Catalog            catalog.Catalog
	Manager            *catalog.Manager
	ProviderRuntime    *ProviderRuntime
	ProviderRegistry   *ProviderRegistryRuntime
	IntegrationRuntime *IntegrationRuntime
	Ops                *OpsState
	GovernanceRuntime  *GovernanceRuntime
	ProbeRuntime       *ProbeRuntime
	ProbeBatchRuntime  *ProbeBatchRuntime
	RoutingService     *routing.Service
	GovernanceRules    *GovernanceRulesRuntime
	ModelAdmissions    *ModelAdmissionRuntime
	ReviewerConnection *ReviewerConnectionRuntime
	Governance         []governance.Record
	Quota              []quota.Snapshot
	QuotaManager       *quota.Manager
	QuotaRuntime       *QuotaRuntime
	RuntimeLearning    any
	Runtime            *Runtime
	ClientKeys         *ClientKeys
	ClientKeyStore     *clientkey.Store
	PublicBaseURL      string
	DataDir            string
}

func (s *Server) json(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func (s *Server) guard(p auth.Permission, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.Auth.Check(r, p) {
			http.Error(w, "unauthorized", 401)
			return
		}
		next(w, r)
	}
}
func (s *Server) current() catalog.Catalog {
	if s.Manager != nil {
		return s.Manager.Snapshot()
	}
	return s.Catalog
}
func (s *Server) Routes() http.Handler {
	m := http.NewServeMux()
	dataDir := s.DataDir
	if dataDir == "" {
		dataDir = "/data"
	}
	auditPath := filepath.Join(dataDir, "audit-xing-shu.jsonl")
	alertsPath := filepath.Join(dataDir, "alerts.jsonl")
	m.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	m.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		c := s.current()
		if len(c.Models) == 0 {
			http.Error(w, "not ready", 503)
			return
		}
		s.json(w, map[string]any{"ready": true, "models": len(c.Models), "stale": []string{}})
	})
	m.HandleFunc("/api/admin/login", func(w http.ResponseWriter, r *http.Request) {
		if !s.Auth.Login(w, r) {
			http.Error(w, "unauthorized", 401)
			return
		}
		s.json(w, map[string]bool{"ok": true})
	})
	m.HandleFunc("/api/admin/logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "xing_shu_admin", Value: "", Path: "/", HttpOnly: true, MaxAge: -1, SameSite: http.SameSiteStrictMode})
		s.json(w, map[string]bool{"ok": true})
	})
	m.Handle("/api/admin/models", s.guard(auth.Read, func(w http.ResponseWriter, r *http.Request) { s.json(w, s.current()) }))
	if s.ClientKeys != nil && s.ClientKeyStore != nil {
		m.Handle("/api/admin/client-api/info", s.guard(auth.Read, ClientAPIInfo(s.PublicBaseURL, s.ClientKeyStore)))
		m.Handle("/api/admin/client-keys", s.guard(auth.Read, s.ClientKeys.List))
		m.Handle("/api/admin/client-keys/create", s.guard(auth.Operate, s.ClientKeys.Create))
		m.Handle("/api/admin/client-keys/enable", s.guard(auth.Operate, s.ClientKeys.Enable))
		m.Handle("/api/admin/client-keys/disable", s.guard(auth.Operate, s.ClientKeys.Disable))
		m.Handle("/api/admin/client-keys/revoke", s.guard(auth.Operate, s.ClientKeys.Revoke))
		m.Handle("/api/admin/client-keys/rotate", s.guard(auth.Operate, s.ClientKeys.Rotate))
		m.Handle("/api/admin/client-keys/mode", s.guard(auth.Operate, s.ClientKeys.Mode))
	}
	m.Handle("/api/admin/governance", s.guard(auth.Read, func(w http.ResponseWriter, r *http.Request) {
		s.json(w, map[string]any{"items": reconcileGovernance(s.current(), s.Governance)})
	}))
	m.Handle("/api/admin/quota", s.guard(auth.Read, func(w http.ResponseWriter, r *http.Request) {
		items := s.Quota
		if s.QuotaManager != nil {
			items = s.QuotaManager.All()
		}
		if items == nil {
			items = []quota.Snapshot{}
		}
		available := quota.HasFacts(items)
		s.json(w, map[string]any{"items": items, "available": available, "message": func() string {
			if !available {
				return "Provider has not supplied quota data"
			}
			return ""
		}()})
	}))
	m.Handle("/api/admin/quota/history", s.guard(auth.Read, func(w http.ResponseWriter, r *http.Request) {
		if s.QuotaManager == nil {
			s.json(w, map[string]any{"items": []quota.Snapshot{}, "available": false, "message": "Provider has not supplied quota data"})
			return
		}
		s.json(w, map[string]any{"items": s.QuotaManager.History(), "available": true})
	}))
	if s.QuotaRuntime != nil {
		m.Handle("/api/admin/quota/status", s.guard(auth.Read, s.QuotaRuntime.Status))
		m.Handle("/api/admin/quota/refresh", s.guard(auth.Probe, s.QuotaRuntime.Refresh))
	}
	m.Handle("/api/admin/overview", s.guard(auth.Read, OverviewView(s.Manager, func() ProviderRuntime {
		if s.ProviderRuntime == nil {
			return ProviderRuntime{}
		}
		return *s.ProviderRuntime
	}(), s.Governance, auditPath, alertsPath)))
	m.Handle("/api/admin/status", s.guard(auth.Read, func(w http.ResponseWriter, r *http.Request) {
		c := s.current()
		x := CatalogStats(c)
		x["version"] = c.Version
		s.json(w, x)
	}))
	m.Handle("/api/admin/runtime", s.guard(auth.Read, func(w http.ResponseWriter, r *http.Request) {
		if s.RuntimeLearning != nil {
			if v, ok := s.RuntimeLearning.(*runtime.Learning); ok {
				RuntimeHandler(RuntimeView{Learning: v}).ServeHTTP(w, r)
				return
			}
		}
		s.json(w, map[string]any{"items": []any{}})
	}))
	m.Handle("/api/admin/recent", s.guard(auth.Read, AuditView(auditPath)))
	m.Handle("/api/admin/diagnostics", s.guard(auth.Read, RuntimeDiagnostics(dataDir)))
	m.Handle("/api/admin/watchdog", s.guard(auth.Read, WatchdogStatusView("")))
	m.Handle("/api/admin/learning", s.guard(auth.Read, LearningView(s.RuntimeLearning)))
	if s.Runtime != nil && s.Runtime.Reviews != nil {
		m.Handle("/api/admin/reviews", s.guard(auth.Read, ReviewsView(s.Runtime.Reviews)))
		m.Handle("/api/admin/reviews/detail", s.guard(auth.Read, ReviewDetailView(s.Runtime.Reviews, 64*1024)))
		m.Handle("/api/admin/reviews/batches", s.guard(auth.Read, ReviewBatchesView(s.Runtime.Reviews)))
		m.Handle("/api/admin/reviews/pending", s.guard(auth.Read, ReviewPendingOnly(s.Runtime.Reviews)))
		m.Handle("/api/admin/reviews/run", s.guard(auth.Operate, ReviewRun(s.Runtime, s.Manager, s.Ops)))
		m.Handle("/api/admin/reviews/stats", s.guard(auth.Read, ReviewStatsView(s.Runtime.Reviews)))
		m.Handle("/api/admin/reviews/prune", s.guard(auth.Operate, ReviewPrune(s.Runtime)))
		m.Handle("/api/admin/knowledge", s.guard(auth.Read, KnowledgeView(s.Runtime.Knowledge)))
		m.Handle("/api/admin/knowledge/build", s.guard(auth.Operate, KnowledgeBuild(s.Runtime)))
		m.Handle("/api/admin/knowledge/decision", s.guard(auth.Operate, KnowledgeDecision(s.Runtime)))
		m.Handle("/api/admin/knowledge/restore", s.guard(auth.Operate, KnowledgeRestore(s.Runtime)))
		m.Handle("/api/admin/knowledge/validate", s.guard(auth.Operate, KnowledgeValidate(s.Runtime, s.Manager, s.Ops)))
		m.Handle("/api/admin/knowledge/models", s.guard(auth.Read, func(w http.ResponseWriter, r *http.Request) {
			s.json(w, map[string]any{"items": KnowledgeModels(s.Manager, s.Ops)})
		}))
		m.Handle("/api/admin/shadow", s.guard(auth.Read, ShadowView(s.Runtime.Shadow)))
		m.Handle("/api/admin/shadow/run", s.guard(auth.Operate, ShadowRun(s.Runtime, s.Manager, s.Ops)))
		m.Handle("/api/admin/shadow/batches", s.guard(auth.Read, ShadowBatchesView(s.Runtime.ShadowBatches)))
		m.Handle("/api/admin/shadow/batches/detail", s.guard(auth.Read, ShadowBatchDetail(s.Runtime.ShadowBatches)))
		m.Handle("/api/admin/shadow/batches/start", s.guard(auth.Operate, ShadowBatchStart(s.Runtime, s.Manager, s.Ops, s.Runtime.ShadowBatches)))
		m.Handle("/api/admin/shadow/batches/stop", s.guard(auth.Operate, ShadowBatchStop(s.Runtime.ShadowBatches)))
		m.Handle("/api/admin/shadow/batches/retry", s.guard(auth.Operate, ShadowBatchRetry(s.Runtime, s.Manager, s.Ops, s.Runtime.ShadowBatches)))
		m.Handle("/api/admin/reviewer", s.guard(auth.Read, ReviewerSelectionView(s.Runtime, s.Manager, s.Ops)))
		m.Handle("/api/admin/reviewer/candidates", s.guard(auth.Read, ReviewerCandidatesView(s.Manager, s.Ops, s.Runtime)))
		m.Handle("/api/admin/reviewer/select", s.guard(auth.Operate, ReviewerSelectionSet(s.Runtime, s.Manager, s.Ops)))
		m.Handle("/api/admin/reviewer/clear", s.guard(auth.Operate, ReviewerSelectionClear(s.Runtime)))
	}
	m.Handle("/api/admin/alerts", s.guard(auth.Read, AlertsView(alertsPath)))
	m.Handle("/api/admin/alerts/action", s.guard(auth.Operate, AlertAction(alertsPath)))
	if s.ModelAdmissions != nil {
		m.Handle("/api/admin/model-admissions/preview", s.guard(auth.Operate, s.ModelAdmissions.Preview))
		m.Handle("/api/admin/model-admissions/apply", s.guard(auth.Operate, s.ModelAdmissions.Apply))
		m.Handle("/api/admin/model-admissions/undo", s.guard(auth.Operate, s.ModelAdmissions.Undo))
		m.Handle("/api/admin/model-admissions/audit", s.guard(auth.Read, s.ModelAdmissions.Audit))
	}
	if s.ProviderRegistry != nil {
		m.Handle("/api/admin/provider-registry", s.guard(auth.Read, s.ProviderRegistry.List))
		m.Handle("/api/admin/provider-registry/validate", s.guard(auth.Probe, s.ProviderRegistry.Validate))
		m.Handle("/api/admin/provider-registry/create", s.guard(auth.Operate, s.ProviderRegistry.Create))
		m.Handle("/api/admin/provider-registry/update", s.guard(auth.Operate, s.ProviderRegistry.Update))
		m.Handle("/api/admin/provider-registry/enable", s.guard(auth.Operate, s.ProviderRegistry.Enable))
		m.Handle("/api/admin/provider-registry/disable", s.guard(auth.Operate, s.ProviderRegistry.Disable))
		m.Handle("/api/admin/provider-registry/delete", s.guard(auth.Operate, s.ProviderRegistry.Delete))
	}
	if s.ProviderRuntime != nil {
		m.Handle("/api/admin/providers", s.guard(auth.Read, s.ProviderRuntime.Handler))
		m.Handle("/api/admin/sync", s.guard(auth.Operate, s.ProviderRuntime.Sync))
	}
	if s.IntegrationRuntime != nil {
		m.Handle("/api/admin/integrations", s.guard(auth.Read, s.IntegrationRuntime.status))
		m.Handle("/api/admin/integrations/freellmapi/status", s.guard(auth.Read, s.IntegrationRuntime.status))
		m.Handle("/api/admin/integrations/freellmapi/probe", s.guard(auth.Probe, s.IntegrationRuntime.Probe))
		m.Handle("/api/admin/integrations/freellmapi/authorize", s.guard(auth.Operate, s.IntegrationRuntime.Authorize))
		m.Handle("/api/admin/integrations/freellmapi/revoke", s.guard(auth.Operate, s.IntegrationRuntime.Revoke))
		m.Handle("/api/admin/integrations/freellmapi/route", s.guard(auth.Operate, s.IntegrationRuntime.Route))
		m.Handle("/api/admin/integrations/freellmapi/sync", s.guard(auth.Operate, s.IntegrationRuntime.Sync))
		m.Handle("/api/admin/integrations/freellmapi/quota/refresh", s.guard(auth.Probe, s.IntegrationRuntime.QuotaRefresh))
		m.Handle("/api/admin/integrations/freellmapi/local-quota", s.guard(auth.Read, s.IntegrationRuntime.LocalQuota))
		m.Handle("/api/admin/integrations/freellmapi/local-quota/refresh", s.guard(auth.Probe, s.IntegrationRuntime.LocalQuotaRefresh))
	}
	if s.ProbeRuntime != nil {
		m.Handle("/api/admin/probe", s.guard(auth.Probe, s.ProbeRuntime.Probe))
	}
	if s.ProbeBatchRuntime != nil {
		m.Handle("/api/admin/probe/batch", s.guard(auth.Probe, s.ProbeBatchRuntime.Start))
		m.Handle("/api/admin/probe/batch/status", s.guard(auth.Probe, s.ProbeBatchRuntime.Status))
		m.Handle("/api/admin/probe/batch/history", s.guard(auth.Read, s.ProbeBatchRuntime.History))
	}
	if s.Ops != nil {
		for _, p := range []string{"/api/admin/cooldowns/clear", "/api/admin/provider/disable", "/api/admin/provider/enable"} {
			m.Handle(p, s.guard(auth.Operate, s.Ops.Handle))
		}
	}
	if s.ReviewerConnection != nil {
		m.Handle("/api/admin/reviewer/test", s.guard(auth.Probe, s.ReviewerConnection.Test))
	}
	if s.GovernanceRules != nil {
		m.Handle("/api/admin/governance/rules/preview", s.guard(auth.Operate, s.GovernanceRules.Preview))
		m.Handle("/api/admin/governance/rules/apply", s.guard(auth.Operate, s.GovernanceRules.Apply))
		m.Handle("/api/admin/governance/rules/undo", s.guard(auth.Operate, s.GovernanceRules.Undo))
		m.Handle("/api/admin/governance/rules/audit", s.guard(auth.Read, s.GovernanceRules.AuditView))
	}
	if s.GovernanceRuntime != nil {
		m.Handle("/api/admin/snapshots", s.guard(auth.Read, s.GovernanceRuntime.List))
		m.Handle("/api/admin/snapshots/detail", s.guard(auth.Read, s.GovernanceRuntime.Detail))
		m.Handle("/api/admin/snapshots/save", s.guard(auth.Operate, s.GovernanceRuntime.Save))
		m.Handle("/api/admin/snapshots/restore", s.guard(auth.Restore, s.GovernanceRuntime.Restore))
	}
	m.Handle("/api/admin/profiles", s.guard(auth.Read, NativeProfiles(s.Manager)))
	if s.RoutingService != nil {
		m.Handle("/api/admin/routing/explain", s.guard(auth.Read, RoutingExplain(s.RoutingService)))
	} else {
		var knowledge *runtime.KnowledgeStore
		if s.Runtime != nil {
			knowledge = s.Runtime.Knowledge
		}
		m.Handle("/api/admin/routing/explain", s.guard(auth.Read, NativeRoutingWithKnowledge(s.Manager, knowledge)))
	}
	m.Handle("/api/admin/accounts", s.guard(auth.Read, NativeAccounts(func() map[string]provider.Config {
		if s.ProviderRuntime == nil {
			return map[string]provider.Config{}
		}
		return s.ProviderRuntime.configs()
	}(), s.Manager, s.Ops)))
	m.Handle("/api/admin/consistency", s.guard(auth.Read, NativeConsistency(s.Manager, s.Governance)))
	m.Handle("/api/admin/regression", s.guard(auth.Read, NativeRegression(s.Manager)))
	return m
}
