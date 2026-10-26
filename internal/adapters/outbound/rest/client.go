// Package rest is the outbound adapter that performs the BFF's upstream HTTP
// calls. It never logs or retains request or response bodies.
package rest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

// maxReplyBytes caps an upstream answer the BFF will relay.
const maxReplyBytes = 4 << 20

// Client implements ports.Upstream over net/http.
type Client struct {
	bases map[composition.Service]*url.URL
	http  *http.Client
}

// New builds a client from one base URL per upstream service. Every service
// must have an absolute http(s) URL. Redirects are never followed and there is
// no client-level timeout: the per-route context budget is the only clock.
func New(bases map[composition.Service]string) (*Client, error) {
	parsed := make(map[composition.Service]*url.URL, len(bases))
	for _, s := range composition.Services() {
		raw := bases[s]
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("base URL of %s is missing or not an absolute http(s) URL", s)
		}
		parsed[s] = u
	}
	hc := &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			MaxIdleConnsPerHost:   32,
			IdleConnTimeout:       90 * time.Second,
			ResponseHeaderTimeout: 0,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &Client{bases: parsed, http: hc}, nil
}

// Do performs the call. A non-2xx answer is returned as a Reply, not an error.
func (c *Client) Do(ctx context.Context, call composition.Call) (composition.Reply, error) {
	base, ok := c.bases[call.Service]
	if !ok {
		return composition.Reply{}, composition.UpstreamError(composition.ErrRouteNotAllowed, call.Service)
	}
	u := *base
	u.Path = joinPath(base.Path, call.Path)
	u.RawQuery = call.Query.Encode()
	var body io.Reader
	if len(call.Body) > 0 {
		body = bytes.NewReader(call.Body)
	}
	req, err := http.NewRequestWithContext(ctx, call.Method, u.String(), body)
	if err != nil {
		return composition.Reply{}, err
	}
	req.Header.Set("Accept", "application/json, application/problem+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if call.IdempotencyKey != "" {
		req.Header.Set("Idempotency-Key", call.IdempotencyKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return composition.Reply{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReplyBytes+1))
	if err != nil {
		return composition.Reply{}, err
	}
	if len(data) > maxReplyBytes {
		return composition.Reply{}, errors.New("upstream reply exceeds the size cap")
	}
	return composition.Reply{Status: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), Body: data}, nil
}

func joinPath(base, p string) string {
	for len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	return base + p
}
