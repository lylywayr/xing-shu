package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type RawModel struct {
	ID                    string `json:"id"`
	Context               int    `json:"context_window"`
	Tools                 bool   `json:"tools"`
	Vision                bool   `json:"vision"`
	StructuredOutput      bool   `json:"structured_output"`
	JSONMode              bool   `json:"json_mode"`
	StructuredOutputKnown bool   `json:"structured_output_known"`
}
type upstreamModels struct {
	Data []RawModel `json:"data"`
}

func FetchModels(ctx context.Context, c Config) ([]RawModel, Result) {
	req, e := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(c.BaseURL, "/")+"/models", nil)
	if e != nil {
		return nil, Result{ErrorType: NetworkError, Message: e.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	r, e := http.DefaultClient.Do(req)
	if e != nil {
		return nil, Result{ErrorType: NetworkError, Message: e.Error()}
	}
	defer r.Body.Close()
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return nil, Result{Status: r.StatusCode, ErrorType: Classify(r.StatusCode, nil)}
	}
	var x upstreamModels
	if e = json.NewDecoder(r.Body).Decode(&x); e != nil {
		return nil, Result{Status: r.StatusCode, ErrorType: InvalidCatalog, Message: e.Error()}
	}
	if len(x.Data) > 5000 {
		return nil, Result{Status: r.StatusCode, ErrorType: InvalidCatalog, Message: "catalog too large"}
	}
	return x.Data, Result{Status: r.StatusCode, LastSuccess: time.Now()}
}
