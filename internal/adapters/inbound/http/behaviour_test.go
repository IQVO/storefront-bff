package http

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

func TestCreateCartSetsStateCookie(t *testing.T) {
	g := newRig()
	g.up.replies["POST /carts"] = reply(201, `{"cartId":"c-42"}`)
	rec := g.do("POST", "/carts", `{}`, "")
	if rec.Code != 201 {
		t.Fatalf("status %d", rec.Code)
	}
	cs := rec.Result().Cookies()
	if len(cs) != 1 {
		t.Fatalf("cookies = %v", cs)
	}
	c := cs[0]
	if c.Name != composition.CookieName || c.Value != "c=c-42" || !c.HttpOnly || !c.Secure || c.Path != BasePath || c.SameSite != http.SameSiteLaxMode || c.MaxAge <= 0 {
		t.Fatalf("cookie = %+v", c)
	}
}

func TestUpstreamProblemsPassThrough(t *testing.T) {
	g := newRig()
	g.up.replies["GET /carts/c1"] = composition.Reply{Status: 404, ContentType: "application/problem+json", Body: []byte(`{"type":"/problems/not-found"}`)}
	rec := g.do("GET", "/carts/c1", "", "")
	if rec.Code != 404 || rec.Header().Get("Content-Type") != "application/problem+json" || rec.Body.String() != `{"type":"/problems/not-found"}` {
		t.Fatalf("%d %s %s", rec.Code, rec.Header(), rec.Body)
	}
	g.up.replies["GET /carts/c2"] = composition.Reply{Status: 204}
	if rec = g.do("GET", "/carts/c2", "", ""); rec.Code != 204 || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("empty reply: %d %v", rec.Code, rec.Header())
	}
}

func TestErrorsBecomeProblems(t *testing.T) {
	cases := []struct {
		err    error
		status int
		slug   string
	}{
		{context.DeadlineExceeded, 504, "gateway-timeout"},
		{errors.New("boom"), 503, "upstream-unavailable"},
		{context.Canceled, 499, "client-closed-request"},
	}
	for _, c := range cases {
		g := newRig()
		g.up.err = c.err
		rec := g.do("GET", "/products/p1", "", "")
		if rec.Code != c.status || !strings.Contains(rec.Body.String(), problemBase+c.slug) || rec.Header().Get("Content-Type") != "application/problem+json" {
			t.Errorf("%v -> %d %s", c.err, rec.Code, rec.Body)
		}
	}
}

func TestEdgeValidationAndSessionProblems(t *testing.T) {
	g := newRig()
	rec := g.do("POST", "/checkout/k1:confirm", `{"paymentMethodToken":"4111111111111111"}`, stateCookie)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "validation-failed") || strings.Contains(rec.Body.String(), "4111") {
		t.Fatalf("card number: %d %s", rec.Code, rec.Body)
	}
	if rec = g.do("GET", "/me", "", ""); rec.Code != 422 || !strings.Contains(rec.Body.String(), "session-required") {
		t.Fatalf("me without cookie: %d %s", rec.Code, rec.Body)
	}
	if len(g.up.calls) != 0 {
		t.Fatal("no upstream call may happen for a refused request")
	}
}

func TestRouteErrors(t *testing.T) {
	g := newRig()
	if rec := g.do("GET", "/nope", "", ""); rec.Code != 404 || !strings.Contains(rec.Body.String(), "resource-not-found") {
		t.Errorf("404: %d %s", rec.Code, rec.Body)
	}
	if rec := g.do("PUT", "/carts", "", ""); rec.Code != 405 {
		t.Errorf("405: %d", rec.Code)
	}
	if rec := g.do("GET", BasePath, "", ""); rec.Code != 404 {
		t.Errorf("bare prefix: %d", rec.Code)
	}
	if rec := g.do("GET", BasePath+"x/products", "", ""); rec.Code != 404 {
		t.Errorf("look-alike prefix: %d", rec.Code)
	}
}

func TestHealthEndpoints(t *testing.T) {
	g := newRig()
	for _, p := range []string{"/healthz", "/readyz", BasePath + "/healthz"} {
		if rec := g.do("GET", p, "", ""); rec.Code != 200 {
			t.Errorf("%s: %d", p, rec.Code)
		}
	}
}

func TestNULAndOversizeBodiesAreRefused(t *testing.T) {
	g := newRig()
	cases := []struct {
		name, target, body string
		status             int
	}{
		{"path", "/products/a%00b", "", 404},
		{"query", "/products?q=a%00b", "", 400},
		{"body escape", "/carts", `{"x":"\u0000"}`, 400},
		{"body raw", "/carts", "{\"x\":\"a\x00b\"}", 400},
		{"too large", "/carts", `{"x":"` + strings.Repeat("a", maxBodyBytes) + `"}`, 413},
	}
	for _, c := range cases {
		method := "GET"
		if c.body != "" {
			method = "POST"
		}
		if rec := g.do(method, c.target, c.body, ""); rec.Code != c.status {
			t.Errorf("%s: %d %s", c.name, rec.Code, rec.Body)
		}
	}
	if len(g.up.calls) != 0 {
		t.Fatal("refused requests must not reach an upstream")
	}
}

func TestPanicBecomes500(t *testing.T) {
	g := newRig()
	h := recoverPanic(g.logsLogger())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("secret body data") }))
	rec := newRecorder()
	h.ServeHTTP(rec, newRequest("POST", "/x"))
	if rec.Code != 500 || strings.Contains(g.logs.String(), "secret") {
		t.Fatalf("%d logs=%s", rec.Code, g.logs)
	}
}

func TestAccessLogCarriesNoBodyQueryOrCookie(t *testing.T) {
	g := newRig()
	g.do("POST", "/shoppers?email=leak@example.test", `{"email":"secret@example.test","savedAddresses":[{"line1":"9 Hidden Rd"}]}`, "c=c-secret")
	out := g.logs.String()
	for _, banned := range []string{"secret", "Hidden", "leak@", "c-secret", "email"} {
		if strings.Contains(out, banned) {
			t.Fatalf("log leaks %q: %s", banned, out)
		}
	}
	if !strings.Contains(out, "route=/shoppers") || !strings.Contains(out, "status=200") {
		t.Fatalf("log = %s", out)
	}
}

func TestBodyAndAddressesReachUpstreamUntouched(t *testing.T) {
	g := newRig()
	body := `{"line1":"9 Hidden Rd","postal":"787"}`
	g.do("POST", "/me/addresses", body, stateCookie)
	if string(g.up.last().Body) != body {
		t.Fatalf("body = %s", g.up.last().Body)
	}
}
