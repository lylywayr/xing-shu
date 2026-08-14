package provider

import (
	"context"
	"errors"
	"net/http"
	"time"
)

type ErrorType string

const (
	NetworkError   ErrorType = "network"
	Unauthorized   ErrorType = "unauthorized"
	RateLimited    ErrorType = "rate_limited"
	Upstream5xx    ErrorType = "upstream_5xx"
	InvalidCatalog ErrorType = "invalid_catalog"
)

type Config struct {
	ID      string
	BaseURL string
	APIKey  string
	Kind    string
}
type Result struct {
	Status      int
	ErrorType   ErrorType
	RetryAfter  time.Duration
	LastSuccess time.Time
	Message     string
}

func Classify(status int, err error) ErrorType {
	if err != nil {
		return NetworkError
	}
	if status == 401 || status == 403 {
		return Unauthorized
	}
	if status == 429 {
		return RateLimited
	}
	if status >= 500 {
		return Upstream5xx
	}
	return InvalidCatalog
}
func Fetch(ctx context.Context, c Config) Result {
	req, e := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/models", nil)
	if e != nil {
		return Result{ErrorType: NetworkError, Message: e.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	r, e := http.DefaultClient.Do(req)
	if e != nil {
		return Result{ErrorType: Classify(0, e), Message: e.Error()}
	}
	defer r.Body.Close()
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return Result{Status: r.StatusCode, ErrorType: Classify(r.StatusCode, nil)}
	}
	return Result{Status: r.StatusCode, LastSuccess: time.Now()}
}

var ErrCatalogEmpty = errors.New("empty catalog")
