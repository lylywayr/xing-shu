package api

import (
	"encoding/json"
	"net/http"
	"xing-shu/internal/runtime"
)

type RuntimeView struct{ Learning *runtime.Learning }

func RuntimeHandler(v RuntimeView) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if v.Learning == nil {
			json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"items": v.Learning.Snapshot()})
	})
}
