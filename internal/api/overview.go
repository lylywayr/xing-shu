package api

import (
	"net/http"
	"time"
	"xing-shu/internal/catalog"
	"xing-shu/internal/governance"
)

type healthProbe struct {
	Status string `json:"status"`
	URL    string `json:"url,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type overviewResponse struct {
	Health struct {
		Service healthProbe `json:"service"`
		Overall string      `json:"overall"`
	} `json:"health"`
	Consistency map[string]any   `json:"consistency"`
	Providers   []map[string]any `json:"providers"`
	Recent      map[string]any   `json:"recent"`
	Alerts      map[string]any   `json:"alerts"`
	GeneratedAt time.Time        `json:"generated_at"`
}

func providerHealth(items []map[string]any) bool {
	for _, item := range items {
		if item["status"] == "disabled" {
			return false
		}
	}
	return true
}

func OverviewView(manager *catalog.Manager, providers ProviderRuntime, governanceRecords []governance.Record, auditPath, alertsPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		catalogSnapshot := catalog.Catalog{}
		if manager != nil {
			catalogSnapshot = manager.Snapshot()
		}
		providerItems := providers.List()
		service := healthProbe{Status: "unhealthy", Detail: "星枢目录为空"}
		if len(catalogSnapshot.Models) > 0 {
			service.Status = "ready"
			service.Detail = "星枢目录已就绪"
		}
		consistency := map[string]any{"catalog": len(catalogSnapshot.Models), "governance": 0, "missing_governance": len(catalogSnapshot.Models), "ok": false}
		if manager != nil {
			reconciled := reconcileGovernance(catalogSnapshot, governanceRecords)
			known := make(map[string]bool, len(reconciled))
			for _, item := range reconciled {
				known[item.Key] = true
			}
			missing := 0
			for _, model := range catalogSnapshot.Models {
				if !known[model.Provider+"/"+model.ID] {
					missing++
				}
			}
			consistency = map[string]any{"catalog": len(catalogSnapshot.Models), "governance": len(reconciled), "missing_governance": missing, "ok": missing == 0}
		}
		recent := ReadJSONL(auditPath)
		alerts := ReadJSONL(alertsPath)
		overall := "ready"
		if service.Status != "ready" || !consistency["ok"].(bool) || !providerHealth(providerItems) {
			overall = "degraded"
			if service.Status == "ready" {
				service.Status = "degraded"
				service.Detail = "星枢部分运行状态需要检查"
			}
		}
		response := overviewResponse{}
		response.Health.Service = service
		response.Health.Overall = overall
		response.Consistency = consistency
		response.Providers = providerItems
		response.Recent = map[string]any{"count": len(recent), "items": tailItems(recent, 5)}
		response.Alerts = map[string]any{"count": len(alerts), "items": tailItems(alerts, 5)}
		response.GeneratedAt = time.Now()
		writeJSON(w, response)
	}
}

func tailItems(items []map[string]any, max int) []map[string]any {
	if len(items) <= max {
		return items
	}
	return items[len(items)-max:]
}
