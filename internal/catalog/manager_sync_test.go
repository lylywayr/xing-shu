package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"xing-shu/internal/provider"
)

func TestSyncPreservesRuntimeCapabilityEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"teacher","context_window":128000}]}`))
	}))
	defer server.Close()
	old := Model{ID: "teacher", Provider: "p", Status: Active, AutoRoutable: true, StructuredOutput: true, StructuredOutputKnown: true}
	manager := NewManager(Catalog{Models: []Model{old}}, map[string]bool{"p/teacher": true})
	manager.Sync(context.Background(), provider.Config{ID: "p", BaseURL: server.URL})
	got := manager.Snapshot().Models[0]
	if !got.StructuredOutput || !got.StructuredOutputKnown {
		t.Fatalf("runtime capability was lost: %+v", got)
	}
}
