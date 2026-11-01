//go:build integration

package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// seen is one request an upstream fake received.
type seen struct {
	Method, Path, Query, Key, Body string
}

// fake is an httptest upstream that records what it receives and answers from
// a "METHOD path" table (404 otherwise).
type fake struct {
	srv     *httptest.Server
	mu      sync.Mutex
	seen    []seen
	answers map[string]answer
	delay   time.Duration
}

type answer struct {
	status int
	body   string
}

func newFake(t *testing.T, answers map[string]answer) *fake {
	t.Helper()
	f := &fake{answers: answers}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.seen = append(f.seen, seen{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Idempotency-Key"), string(b)})
		delay := f.delay
		f.mu.Unlock()
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		a, ok := f.answers[r.Method+" "+r.URL.Path]
		if !ok {
			a = answer{404, `{"type":"/problems/not-found","title":"Not found","status":404}`}
		}
		ct := "application/json"
		if a.status >= 400 {
			ct = "application/problem+json"
		}
		w.Header().Set("Content-Type", ct)
		w.WriteHeader(a.status)
		_, _ = io.WriteString(w, a.body)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) requests() []seen {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]seen(nil), f.seen...)
}

func (f *fake) find(method, path string) (seen, bool) {
	for _, s := range f.requests() {
		if s.Method == method && s.Path == path {
			return s, true
		}
	}
	return seen{}, false
}

// bff boots the real binary wiring against the given fakes and returns its
// base URL.
func bff(t *testing.T, ups map[string]*fake, extra map[string]string) string {
	t.Helper()
	env := map[string]string{"PORT": "0", "COOKIE_SECURE": "false"}
	for name, f := range ups {
		env[name] = f.srv.URL
	}
	for k, v := range extra {
		env[k] = v
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready, done := make(chan string, 1), make(chan error, 1)
	go func() { done <- run(ctx, func(k string) string { return env[k] }, quietLog(), ready) }()
	addr := <-ready
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return "http://" + addr + "/api/storefront"
}

type session struct {
	t       *testing.T
	base    string
	cookies []*http.Cookie
}

func (s *session) do(method, path, body string) (int, string) {
	s.t.Helper()
	req, err := http.NewRequest(method, s.base+path, strings.NewReader(body))
	if err != nil {
		s.t.Fatal(err)
	}
	for _, c := range s.cookies {
		req.AddCookie(c)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if cs := resp.Cookies(); len(cs) > 0 {
		s.cookies = cs
	}
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}
