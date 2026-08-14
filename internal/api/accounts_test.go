package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"xing-shu/internal/catalog"
	"xing-shu/internal/provider"
)

func TestNativeAccountsReturnsRedactedProviderStatus(t *testing.T) {
	manager := catalog.NewManager(catalog.Catalog{Models: []catalog.Model{{ID: "m1", Provider: "p1", Status: catalog.Active}}}, nil)
	w := httptest.NewRecorder()
	NativeAccounts(map[string]provider.Config{"p1": {ID: "p1", BaseURL: "http://example", APIKey: "secret", Kind: "credit"}}, manager, NewOps()).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v2/admin/accounts", nil))
	if strings.Contains(w.Body.String(), "secret") {
		t.Fatal("account response leaked API key")
	}
	var body struct {
		Configured bool             `json:"configured"`
		Accounts   []map[string]any `json:"accounts"`
	}
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || !body.Configured || len(body.Accounts) != 1 {
		t.Fatalf("unexpected account status: %s", w.Body.String())
	}
}
