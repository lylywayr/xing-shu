package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"xing-shu/internal/api"
	"xing-shu/internal/auth"
	"xing-shu/internal/catalog"
	"xing-shu/internal/governance"
	"xing-shu/internal/integration"
	"xing-shu/internal/observability"
	"xing-shu/internal/provider"
	"xing-shu/internal/quota"
	"xing-shu/internal/routing"
	"xing-shu/internal/storage"
)

func dataDir() string {
	if value := os.Getenv("STARCORE_DATA_DIR"); value != "" {
		return filepath.Clean(value)
	}
	return "/data"
}

func loadCatalogState(dir string) (catalog.State, error) {
	return catalog.LoadState(filepath.Join(dir, "catalog-starcore.json"))
}

func adminAuthorizer() auth.Authorizer {
	key := os.Getenv("ADMIN_API_KEY")
	user := os.Getenv("STARCORE_ADMIN_USER")
	password := os.Getenv("STARCORE_ADMIN_PASSWORD")
	if user == "" {
		user = "admin"
	}
	if password == "" {
		password = key
	}
	return auth.Authorizer{AdminKey: key, Username: user, Password: password}
}

func main() {
	dir := dataDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		log.Fatalf("starcore data directory: %v", err)
	}
	state, err := loadCatalogState(dir)
	if err != nil {
		log.Fatalf("load starcore catalog state: %v", err)
	}
	gov, err := storage.LoadGovernance(dir)
	if err != nil {
		log.Fatalf("load starcore governance state: %v", err)
	}
	manager := catalog.NewManagerWithState(state)
	manager.SetPersist(func(next catalog.State) {
		if err := catalog.SaveState(filepath.Join(dir, "catalog-starcore.json"), next); err != nil {
			log.Printf("starcore catalog persistence failed: %v", err)
		}
	})
	sharedRuntime := api.NewRuntimeAt(dir)
	configs := provider.LoadConfigs()
	freeManager, _ := integration.New(filepath.Join(dir, "integrations-starcore.json"), os.Getenv("FREELLMAPI_URL"), os.Getenv("FREELLMAPI_KEY"), os.Getenv("STARCORE_FREELLMAPI_MIGRATE_AUTH") == "true")
	if freeManager != nil {
		freeManager.SetLocalQuotaPath(os.Getenv("FREELLMAPI_DB_PATH"))
	}
	if freeManager != nil && freeManager.Configured() {
		provider.RegisterExternalConfig(integration.FreeLLMAPIID, freeManager.Config())
		configs = provider.LoadConfigs()
	}
	if selected := sharedRuntime.ReviewerSelection.Get(); selected.Model != "" {
		if manager.RestoreReviewerCapability(selected.Model, selected.Provider) {
			sharedRuntime.ReviewerSelection.Success()
		}
	}
	syncCtx := context.Background()
	ops := api.NewOps()
	ops.Load(filepath.Join(dir, "provider-ops-starcore.json"))
	catalog.StartSyncWithObserverGated(syncCtx, manager, configs, 60*time.Second, func(providerID string, result provider.Result) {
		if result.ErrorType == "" {
			ops.ClearSyncError(providerID)
			return
		}
		ops.MarkSyncError(providerID, string(result.ErrorType), result.Message)
	}, func(providerID string) bool {
		if providerID == integration.FreeLLMAPIID && freeManager != nil {
			return freeManager.Authorized()
		}
		return true
	})
	probe := &api.ProbeRuntime{Configs: configs, Manager: manager, Gate: map[string]func() bool{}}
	if freeManager != nil {
		probe.Gate[integration.FreeLLMAPIID] = freeManager.Authorized
	}
	probeBatch := api.NewProbeBatchRuntime(probe, dir)
	governanceRules := api.NewGovernanceRulesRuntime(manager, dir)
	reviewerConnection := &api.ReviewerConnectionRuntime{Configs: configs, Manager: manager}
	routingService := &routing.Service{Providers: map[string]routing.ProviderConfig{}, ProviderGate: map[string]func() bool{}, Models: manager.Snapshot().Models, Manager: manager, Disabled: ops, Client: provider.NewChatClient(), Knowledge: sharedRuntime.Knowledge}
	for id, c := range configs {
		routingService.Providers[id] = routing.ProviderConfig{ID: c.ID, BaseURL: c.BaseURL, APIKey: c.APIKey, Kind: c.Kind}
	}
	if freeManager != nil {
		routingService.ProviderGate[integration.FreeLLMAPIID] = freeManager.RouteEnabled
	}
	api.StartReviewWorkerOnce(syncCtx, sharedRuntime, manager, ops, 24*time.Hour)
	api.StartShadowWorker(syncCtx, sharedRuntime, manager, ops, 24*time.Hour)
	api.StartMaintenance(syncCtx, sharedRuntime)
	snap := governance.NewManager(filepath.Join(dir, "snapshots-starcore"))
	_ = snap.SaveCatalog(manager.Snapshot())
	quotaManager := quota.NewManager()
	if freeManager != nil {
		routingService.QuotaProvider = quotaManager.Get
	}
	integrationRuntime := &api.IntegrationRuntime{FreeLLMAPI: freeManager, Catalog: manager, Quota: quotaManager, Audit: observability.New(dir)}
	if freeManager != nil {
		integrationRuntime.Background(syncCtx, 5*time.Minute)
	}
	quotaRuntime := api.NewQuotaRuntimeFromEnvWithConfigs(quotaManager, configs)
	providerRuntime := &api.ProviderRuntime{Configs: configs, Manager: manager, Ops: ops, SyncGate: map[string]func() bool{}}
	if freeManager != nil {
		providerRuntime.SyncGate[integration.FreeLLMAPIID] = freeManager.Authorized
	}
	s := &api.Server{Auth: adminAuthorizer(), Catalog: state.Catalog, Manager: manager, ProviderRuntime: providerRuntime, IntegrationRuntime: integrationRuntime, Ops: ops, ProbeRuntime: probe, ProbeBatchRuntime: probeBatch, GovernanceRules: governanceRules, ReviewerConnection: reviewerConnection, GovernanceRuntime: &api.GovernanceRuntime{Snapshots: snap, Catalog: manager}, QuotaManager: quotaManager, QuotaRuntime: quotaRuntime, RuntimeLearning: sharedRuntime.Learning, Runtime: sharedRuntime, RoutingService: routingService, DataDir: dir, Governance: gov}
	mux := http.NewServeMux()
	mux.Handle("/", staticHandler(http.FileServer(http.Dir("/app/web"))))
	mux.Handle("/admin/ui", http.RedirectHandler("/", http.StatusFound))
	apiMux := s.Routes()
	mux.Handle("/health", apiMux)
	mux.Handle("/health/", apiMux)
	mux.Handle("/api/", apiMux)
	mux.Handle("/v2/", apiMux)
	mux.Handle("/v1/models", api.NativeModelsManager(manager))
	if os.Getenv("STARCORE_NATIVE_CHAT") != "false" {
		ledger := quota.NewLedger()
		ledger.Load(filepath.Join(dir, "quota-ledger-starcore.json"))
		mux.Handle("/v1/chat/completions", &api.Chat{Router: routingService, Fallback: nil, Audit: observability.New(dir), Ledger: ledger, Runtime: sharedRuntime})
	}
	mux.HandleFunc("/v1/", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	addr := os.Getenv("LISTEN")
	if addr == "" {
		addr = ":12200"
	}
	log.Printf("starcore listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
