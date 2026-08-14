package api

import (
	"net/http"
	"xing-shu/internal/catalog"
)

func CatalogStats(c catalog.Catalog) map[string]any {
	out := map[string]int{"current_catalog": len(c.Models), "auto_routable": 0, "active": 0, "unknown": 0, "meta": 0, "stale": 0, "retired": 0, "disabled": 0}
	for _, m := range c.Models {
		if m.AutoRoutable {
			out["auto_routable"]++
		}
		switch m.Status {
		case catalog.Active:
			out["active"]++
		case catalog.Unknown:
			out["unknown"]++
		case catalog.Meta:
			out["meta"]++
		case catalog.Stale:
			out["stale"]++
		case catalog.Retired:
			out["retired"]++
		case catalog.Disabled:
			out["disabled"]++
		}
	}
	return map[string]any{"stats": out, "public_models": out["auto_routable"] + 1}
}
func StatsHandler(m *catalog.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := m.Snapshot()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stats":{}}`))
		_ = c
	})
}
