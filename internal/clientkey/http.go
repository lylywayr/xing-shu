package clientkey

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

type Mode string

const (
	ModeOptional Mode = "optional"
	ModeRequired Mode = "required"
)

type contextKey struct{}

func FromContext(ctx context.Context) (Key, bool) {
	key, ok := ctx.Value(contextKey{}).(Key)
	return key, ok
}

type Authenticator struct {
	Store *Store
}

func (a Authenticator) Middleware(scope string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.Store == nil {
			http.Error(w, "client API authentication unavailable", http.StatusServiceUnavailable)
			return
		}
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if header == "" && a.Store.Mode() == ModeOptional {
			w.Header().Set("X-Xing-Shu-Auth", "migration-optional")
			next.ServeHTTP(w, r)
			return
		}
		key, err := a.Store.Authenticate(r, scope, time.Now())
		if err != nil {
			status := http.StatusUnauthorized
			if errors.Is(err, ErrForbidden) {
				status = http.StatusForbidden
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": err.Error(), "type": "authentication_error"}})
			return
		}
		w.Header().Set("X-Xing-Shu-Key", key.Prefix)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, key)))
	})
}
