package http

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

// keyedRoutes is every POST and PATCH of the edge, with a valid body.
var keyedRoutes = []struct{ method, target, body string }{
	{"POST", "/products/p1/delivery-date-quotes", `{"postalPrefix":"787"}`},
	{"POST", "/carts", `{}`},
	{"POST", "/carts/c1/lines", `{"catalogItemId":"i1","quantity":1}`},
	{"PATCH", "/carts/c1/lines/i1", `{"quantity":2}`},
	{"POST", "/carts/c1/delivery-quotes", ``},
	{"POST", "/checkout", `{"shipToAddressId":"a1"}`},
	{"POST", "/checkout/k1/confirmations", `{"paymentMethodToken":"tok_1"}`},
	{"POST", "/orders/o1/cancellations", `{"reason":"changed my mind"}`},
	{"POST", "/shoppers", `{"email":"a@example.test"}`},
	{"POST", "/sessions", `{"email":"a@example.test"}`},
	{"POST", "/me/addresses", `{"line1":"1 Test Way"}`},
}

func problemType(t *testing.T, body string) string {
	t.Helper()
	var p struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("not a problem: %s", body)
	}
	return p.Type
}

func TestPostAndPatchRequireIdempotencyKey(t *testing.T) {
	for _, c := range keyedRoutes {
		g := newRig()
		rec := newRecorder()
		g.h.ServeHTTP(rec, g.reqWithKey(c.method, c.target, c.body, ""))
		if rec.Code != 400 || problemType(t, rec.Body.String()) != "/problems/idempotency-key-required" {
			t.Errorf("%s %s without a key -> %d %s", c.method, c.target, rec.Code, rec.Body)
		}
		if len(g.up.calls) != 0 {
			t.Errorf("%s %s without a key reached an upstream", c.method, c.target)
		}
	}
}

func TestInvalidIdempotencyKeyIsRefused(t *testing.T) {
	for _, key := range []string{strings.Repeat("k", 129), "has space"} {
		g := newRig()
		rec := newRecorder()
		g.h.ServeHTTP(rec, g.reqWithKey("POST", "/carts", `{}`, key))
		if rec.Code != 400 || problemType(t, rec.Body.String()) != "/problems/idempotency-key-invalid" {
			t.Errorf("key %q -> %d %s", key, rec.Code, rec.Body)
		}
	}
	g := newRig()
	req := g.reqWithKey("POST", "/carts", `{}`, "k-1")
	req.Header.Add(idempotencyKeyHeader, "k-2")
	rec := newRecorder()
	g.h.ServeHTTP(rec, req)
	if rec.Code != 400 || problemType(t, rec.Body.String()) != "/problems/idempotency-key-invalid" {
		t.Errorf("repeated key -> %d %s", rec.Code, rec.Body)
	}
	ok := newRig()
	rec = newRecorder()
	ok.h.ServeHTTP(rec, ok.reqWithKey("POST", "/carts", `{}`, strings.Repeat("k", 128)))
	if rec.Code != 200 {
		t.Errorf("a 128-character key must be accepted, got %d", rec.Code)
	}
}

// PUT and DELETE take no middleware: a key is optional and never required.
func TestPutAndDeleteDoNotRequireAKey(t *testing.T) {
	for _, c := range []struct{ method, target, body string }{
		{"PUT", "/carts/c1/destination", `{"destinationPostalPrefix":"787"}`},
		{"PUT", "/carts/c1/delivery-selection", `{"groupNo":1,"offerId":"o1"}`},
		{"DELETE", "/carts/c1/lines/i1", ``},
	} {
		g := newRig()
		rec := newRecorder()
		g.h.ServeHTTP(rec, g.reqWithKey(c.method, c.target, c.body, ""))
		if rec.Code != 200 {
			t.Errorf("%s %s without a key -> %d %s", c.method, c.target, rec.Code, rec.Body)
		}
	}
}

func TestReplayedHeaderIsRelayedOnlyOnReplay(t *testing.T) {
	g := newRig()
	g.up.replies["POST /carts/c1/lines"] = composition.Reply{Status: 200, ContentType: "application/json", Body: []byte(`{"cartId":"c1"}`), Replayed: true}
	rec := g.do("POST", "/carts/c1/lines", `{"catalogItemId":"i1","quantity":1}`, stateCookie)
	if rec.Code != 200 || rec.Header().Get(replayedHeader) != "true" || rec.Body.String() != `{"cartId":"c1"}` {
		t.Fatalf("replay -> %d %q %s", rec.Code, rec.Header().Get(replayedHeader), rec.Body)
	}
	g.up.replies["POST /carts/c1/lines"] = reply(200, `{"cartId":"c1"}`)
	rec = g.do("POST", "/carts/c1/lines", `{"catalogItemId":"i1","quantity":1}`, stateCookie)
	if _, set := rec.Header()[replayedHeader]; set {
		t.Fatal("a fresh answer must not carry Idempotency-Replayed")
	}
	// The composed delivery-dates answer is replayed when its quote hop was.
	g.up.replies["POST /delivery-quotes"] = composition.Reply{Status: 200, ContentType: "application/json", Body: []byte(`{"offers":[],"unavailable":[]}`), Replayed: true}
	rec = g.do("POST", "/products/p1/delivery-date-quotes", `{"postalPrefix":"787"}`, "")
	if rec.Code != 200 || rec.Header().Get(replayedHeader) != "true" {
		t.Fatalf("composed replay -> %d %q %s", rec.Code, rec.Header().Get(replayedHeader), rec.Body)
	}
}

// Same page key with a different body: the provider answers 409
// idempotency-key-reused and the BFF passes the problem through unchanged.
func TestKeyReuseConflictFromProviderIsRelayed(t *testing.T) {
	g := newRig()
	problem := `{"type":"/problems/idempotency-key-reused","title":"Idempotency-Key reused","status":409}`
	g.up.replies["POST /orders/o1/cancellations"] = composition.Reply{Status: 409, ContentType: "application/problem+json", Body: []byte(problem)}
	rec := g.do("POST", "/orders/o1/cancellations", `{"reason":"other"}`, stateCookie)
	if rec.Code != 409 || rec.Body.String() != problem || rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("-> %d %s %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body)
	}
}

func TestRetiredRoutesAreGone(t *testing.T) {
	for _, c := range []struct{ method, target, body string }{
		{"POST", "/products/p1:delivery-dates", `{"postalPrefix":"787"}`},
		{"POST", "/carts/c1/destination", `{"destinationPostalPrefix":"787"}`},
		{"POST", "/carts/c1/delivery:quote", ``},
		{"POST", "/carts/c1/delivery:select", `{"groupNo":1,"offerId":"o1"}`},
		{"POST", "/checkout/k1:confirm", `{"paymentMethodToken":"tok_1"}`},
		{"POST", "/orders/o1:cancel", `{"reason":"changed my mind"}`},
	} {
		g := newRig()
		rec := g.do(c.method, c.target, c.body, stateCookie)
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s -> %d, want 404 or 405", c.method, c.target, rec.Code)
		}
		if len(g.up.calls) != 0 {
			t.Errorf("%s %s reached an upstream", c.method, c.target)
		}
	}
}

func TestPagedListsRejectUnknownParamsAndBadLimits(t *testing.T) {
	for _, target := range []string{"/products", "/carts", "/orders"} {
		for _, q := range []string{"after=x", "offset=10", "nextToken=t", "bogus=1"} {
			g := newRig()
			rec := g.do("GET", target+"?"+q, "", stateCookie)
			if rec.Code != 400 || problemType(t, rec.Body.String()) != "/problems/unknown-query-parameter" {
				t.Errorf("GET %s?%s -> %d %s", target, q, rec.Code, rec.Body)
			}
		}
		for _, q := range []string{"limit=0", "limit=501", "limit=x", "limit=", "limit=1&limit=2"} {
			g := newRig()
			rec := g.do("GET", target+"?"+q, "", stateCookie)
			if rec.Code != 400 || problemType(t, rec.Body.String()) != "/problems/invalid-limit" {
				t.Errorf("GET %s?%s -> %d %s", target, q, rec.Code, rec.Body)
			}
		}
		g := newRig()
		rec := g.do("GET", target+"?limit=2&cursor=opaque", "", stateCookie)
		if rec.Code != 200 {
			t.Fatalf("GET %s paged -> %d %s", target, rec.Code, rec.Body)
		}
		if got := g.up.last().Query; got.Get("limit") != "2" || got.Get("cursor") != "opaque" {
			t.Errorf("GET %s forwarded %v, want limit and cursor relayed", target, got)
		}
	}
}
