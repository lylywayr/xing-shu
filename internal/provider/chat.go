package provider

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

type ChatClient struct{ HTTP *http.Client }

func NewChatClient() *ChatClient {
	return &ChatClient{HTTP: &http.Client{Timeout: 120 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (c *ChatClient) Complete(ctx context.Context, base, key string, body []byte) (*http.Response, error) {
	req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(base, "/")+"/chat/completions", bytes.NewReader(body))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	r, e := c.HTTP.Do(req)
	if e != nil {
		return nil, e
	}
	return r, nil
}
func ReadBody(r *http.Response) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(io.LimitReader(r.Body, 32<<20))
}
