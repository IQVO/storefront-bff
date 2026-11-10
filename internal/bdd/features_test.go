// Package bdd runs the Gherkin scenarios of ../../features against the real
// edge router and composition use cases, with a scripted upstream port.
package bdd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/cucumber/godog"

	httpin "github.com/IQVO/storefront-bff/internal/adapters/inbound/http"
	"github.com/IQVO/storefront-bff/internal/application/usecases"
	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

// TestFeatures runs every scenario in ../../features (make bdd).
func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initializeScenario,
		Options:             &godog.Options{Format: "pretty", Paths: []string{"../../features"}, TestingT: t, Strict: true},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}

// scriptedUpstream answers each call with the reply scripted for
// "METHOD path" (200 {} otherwise) and records every call.
type scriptedUpstream struct {
	mu      sync.Mutex
	replies map[string]composition.Reply
	err     error
	calls   []composition.Call
}

func (s *scriptedUpstream) Do(_ context.Context, c composition.Call) (composition.Reply, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, c)
	if s.err != nil {
		return composition.Reply{}, s.err
	}
	if r, ok := s.replies[c.Method+" "+c.Path]; ok {
		return r, nil
	}
	return composition.Reply{Status: http.StatusOK, ContentType: "application/json", Body: []byte(`{}`)}, nil
}

// world is the state of one scenario.
type world struct {
	up     *scriptedUpstream
	edge   http.Handler
	cookie string
	rec    *httptest.ResponseRecorder
}

func newWorld() *world {
	up := &scriptedUpstream{replies: map[string]composition.Reply{}}
	g := usecases.NewGateway(up, composition.DefaultBudgets())
	srv := httpin.NewServer(g, slog.New(slog.NewTextHandler(io.Discard, nil)), true, "")
	return &world{up: up, edge: srv.Handler()}
}

func initializeScenario(sc *godog.ScenarioContext) {
	w := newWorld()
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = *newWorld()
		return ctx, nil
	})
	registerUpstream(sc, w)
	registerPage(sc, w)
	registerEdge(sc, w)
}

func registerUpstream(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^the upstream answers "([A-Z]+ [^"]+)" with status (\d+) and body (.+)$`, func(route string, status int, body string) {
		w.up.replies[route] = composition.Reply{Status: status, ContentType: "application/json", Body: []byte(body)}
	})
	sc.Step(`^the upstream answers "([A-Z]+ [^"]+)" with the problem status (\d+) and body (.+)$`, func(route string, status int, body string) {
		w.up.replies[route] = composition.Reply{Status: status, ContentType: "application/problem+json", Body: []byte(body)}
	})
	sc.Step(`^the upstream answers "([A-Z]+ [^"]+)" from its idempotency store with body (.+)$`, func(route, body string) {
		w.up.replies[route] = composition.Reply{Status: http.StatusOK, ContentType: "application/json", Body: []byte(body), Replayed: true}
	})
	sc.Step(`^the upstream fails with (a timeout|a transport error)$`, func(kind string) {
		if kind == "a timeout" {
			w.up.err = context.DeadlineExceeded
			return
		}
		w.up.err = errors.New("connection refused")
	})
}

func registerPage(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^the page holds the state cookie "([^"]+)"$`, func(v string) { w.cookie = v })
	sc.Step(`^the page sends "(GET|DELETE) ([^"]+)"$`, func(method, target string) { w.send(method, target, "", "") })
	sc.Step(`^the page sends "(POST|PATCH|PUT) ([^"]+)" with body (.+) and an Idempotency-Key$`, func(method, target, body string) {
		w.send(method, target, body, "k-bdd")
	})
	sc.Step(`^the page sends "(POST|PATCH|PUT) ([^"]+)" with body (.+) and no Idempotency-Key$`, func(method, target, body string) {
		w.send(method, target, body, "")
	})
	sc.Step(`^the page sends "(POST|PATCH|PUT) ([^"]+)" with body (.+) and the Idempotency-Key "([^"]+)"$`, w.send)
}

func (w *world) send(method, target, body, key string) {
	var rd io.Reader = http.NoBody
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, rd)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if w.cookie != "" {
		req.AddCookie(&http.Cookie{Name: composition.CookieName, Value: w.cookie})
	}
	w.rec = httptest.NewRecorder()
	w.edge.ServeHTTP(w.rec, req)
}

func registerEdge(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^the edge answers status (\d+)$`, w.statusIs)
	sc.Step(`^the edge answers with body (.+)$`, w.bodyIs)
	sc.Step(`^the edge answers with content type "([^"]+)"$`, func(ct string) error {
		return expect(w.rec.Header().Get("Content-Type") == ct, "content type %q", w.rec.Header().Get("Content-Type"))
	})
	sc.Step(`^the edge answers the header "([^"]+)" as "([^"]+)"$`, func(name, v string) error {
		return expect(w.rec.Header().Get(name) == v, "header %s = %q", name, w.rec.Header().Get(name))
	})
	sc.Step(`^the edge answers the problem "([^"]+)"$`, w.problemIs)
	sc.Step(`^the edge sets the state cookie "([^"]+)" as HttpOnly, Secure and SameSite=Lax on the storefront path$`, w.stateCookieIs)
	sc.Step(`^the last upstream call was "([A-Z]+) ([^"]+)" with no query and no Idempotency-Key$`, w.lastPlainCall)
	sc.Step(`^the last upstream call carries a minted Idempotency-Key that is not "([^"]+)"$`, w.lastCallMinted)
	sc.Step(`^no upstream was called$`, func() error {
		return expect(len(w.up.calls) == 0, "upstream calls = %+v", w.up.calls)
	})
}

func (w *world) statusIs(status int) error {
	return expect(w.rec.Code == status, "status %d, body %s", w.rec.Code, w.rec.Body)
}

func (w *world) bodyIs(body string) error {
	return expect(w.rec.Body.String() == body, "body %s", w.rec.Body)
}

func (w *world) problemIs(slug string) error {
	ok := w.rec.Header().Get("Content-Type") == "application/problem+json" &&
		strings.Contains(w.rec.Body.String(), `"/problems/`+slug+`"`)
	return expect(ok, "not the %s problem: %s %s", slug, w.rec.Header().Get("Content-Type"), w.rec.Body)
}

func (w *world) stateCookieIs(value string) error {
	cs := w.rec.Result().Cookies()
	if len(cs) != 1 {
		return fmt.Errorf("cookies = %v", cs)
	}
	c := cs[0]
	ok := c.Name == composition.CookieName && c.Value == value && c.HttpOnly && c.Secure &&
		c.SameSite == http.SameSiteLaxMode && c.Path == httpin.BasePath && c.MaxAge > 0
	return expect(ok, "cookie = %+v", c)
}

func (w *world) lastCall() (composition.Call, error) {
	if len(w.up.calls) == 0 {
		return composition.Call{}, errors.New("no upstream call was made")
	}
	return w.up.calls[len(w.up.calls)-1], nil
}

func (w *world) lastPlainCall(method, path string) error {
	c, err := w.lastCall()
	if err != nil {
		return err
	}
	ok := c.Method == method && c.Path == path && len(c.Query) == 0 && c.IdempotencyKey == ""
	return expect(ok, "upstream call = %+v", c)
}

func (w *world) lastCallMinted(pageKey string) error {
	c, err := w.lastCall()
	if err != nil {
		return err
	}
	ok := c.IdempotencyKey != pageKey && strings.HasPrefix(c.IdempotencyKey, "sf-") && len(c.IdempotencyKey) == 43
	return expect(ok, "upstream Idempotency-Key = %q", c.IdempotencyKey)
}

func expect(ok bool, format string, args ...any) error {
	if ok {
		return nil
	}
	return fmt.Errorf(format, args...)
}
