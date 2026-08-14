package api

import (
	"encoding/json"
	"net/http"
	"xing-shu/internal/catalog"
)

func ModelsResponse(c catalog.Catalog) map[string]any {
	data := []catalog.Model{{ID: "auto", Provider: "smart-router", Object: "model", OwnedBy: "smart-router", Created: 0, AutoRoutable: true, Capabilities: []string{"general", "quick", "code_generation", "reasoning", "long_context", "multimodal"}}}
	for _, m := range c.Models {
		if m.AutoRoutable && m.Status == catalog.Active && !catalog.IsMeta(m.ID) {
			data = append(data, m)
		}
	}
	return map[string]any{"object": "list", "data": data}
}
func NativeModels(c catalog.Catalog) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ModelsResponse(c))
	})
}
func NativeModelsManager(m *catalog.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ModelsResponse(m.Snapshot()))
	})
}
