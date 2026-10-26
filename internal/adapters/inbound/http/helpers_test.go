package http

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/IQVO/storefront-bff/internal/application/usecases"
	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

// stubUp answers every upstream call with the scripted reply for
// "METHOD path", recording the calls.
type stubUp struct {
	mu      sync.Mutex
	replies map[string]composition.Reply
	err     error
	calls   []composition.Call
}

func (s *stubUp) Do(_ context.Context, c composition.Call) (composition.Reply, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, c)
	if s.err != nil {
		return composition.Reply{}, s.err
	}
	if r, ok := s.replies[c.Method+" "+c.Path]; ok {
		return r, nil
	}
	return composition.Reply{Status: 200, ContentType: "application/json", Body: []byte(`{}`)}, nil
}

func (s *stubUp) last() composition.Call { return s.calls[len(s.calls)-1] }

func reply(status int, body string) composition.Reply {
	return composition.Reply{Status: status, ContentType: "application/json", Body: []byte(body)}
}

type rig struct {
	server *Server
	up     *stubUp
	logs   *bytes.Buffer
	h      http.Handler
}

func newRig() *rig {
	up := &stubUp{replies: map[string]composition.Reply{}}
	logs := &bytes.Buffer{}
	log := slog.New(slog.NewTextHandler(logs, nil))
	g := usecases.NewGateway(up, composition.DefaultBudgets(), func() time.Time { return time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC) })
	srv := NewServer(g, log, true)
	return &rig{server: srv, up: up, logs: logs, h: srv.Handler()}
}

func (g *rig) do(method, target, body string, cookie string) *httptest.ResponseRecorder {
	var rd io.Reader = http.NoBody
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, rd)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: composition.CookieName, Value: cookie})
	}
	rec := httptest.NewRecorder()
	g.h.ServeHTTP(rec, req)
	return rec
}

func newRecorder() *httptest.ResponseRecorder { return httptest.NewRecorder() }

func (g *rig) reqWithAction(method, target, body, action string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("X-Action-Id", action)
	return req
}

func (g *rig) logsLogger() *slog.Logger { return slog.New(slog.NewTextHandler(g.logs, nil)) }

func newRequest(method, target string) *http.Request {
	return httptest.NewRequest(method, target, http.NoBody)
}
