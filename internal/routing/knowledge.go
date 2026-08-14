package routing

import (
	"encoding/json"
	"strings"
	"xing-shu/internal/runtime"
)

type TaskFeatures = runtime.TaskFeatures

func TaskFeaturesFromBody(b []byte) TaskFeatures {
	var x struct {
		Tools          []any `json:"tools"`
		ResponseFormat any   `json:"response_format"`
		Messages       []any `json:"messages"`
	}
	_ = json.Unmarshal(b, &x)
	return runtime.TaskFeatures{NeedsTools: len(x.Tools) > 0, NeedsStructured: x.ResponseFormat != nil, MessageCount: len(x.Messages), InputBytes: len(b)}
}
func HighRisk(b []byte) bool {
	s := strings.ToLower(string(b))
	for _, x := range []string{"production", "deploy", "database", "migration", "credential", "password", "api_key", "docker", "nas", "delete"} {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}
