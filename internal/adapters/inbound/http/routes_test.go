package http

import (
	"testing"
)

const stateCookie = "c=c1&k=k1&s=s1"

func TestEveryRouteMapsToItsUpstreamCall(t *testing.T) {
	cases := []struct {
		method, target, body string
		wantCall             string
	}{
		{"GET", "/products?q=lamp", "", "GET /products"},
		{"GET", "/categories", "", "GET /categories"},
		{"GET", "/products/p1", "", "GET /products/p1"},
		{"POST", "/products/p1:delivery-dates", `{"postalPrefix":"787"}`, "POST /delivery-offers:quote"},
		{"POST", "/carts", `{}`, "POST /carts"},
		{"GET", "/carts?state=OPEN", "", "GET /carts"},
		{"GET", "/carts/c1", "", "GET /carts/c1"},
		{"POST", "/carts/c1/lines", `{"catalogItemId":"i1","quantity":1}`, "POST /carts/c1/lines"},
		{"PATCH", "/carts/c1/lines/i1", `{"quantity":2}`, "POST /carts/c1/lines/i1:set-quantity"},
		{"DELETE", "/carts/c1/lines/i1", "", "POST /carts/c1/lines/i1:remove"},
		{"POST", "/carts/c1/destination", `{"destinationPostalPrefix":"787"}`, "POST /carts/c1:set-destination"},
		{"POST", "/carts/c1/delivery:quote", "", "POST /carts/c1:quote-delivery"},
		{"POST", "/carts/c1/delivery:select", `{"groupNo":1,"offerId":"o1"}`, "POST /carts/c1:select-delivery"},
		{"POST", "/checkout", `{"shipToAddressId":"a1"}`, "POST /checkout-sessions"},
		{"GET", "/checkout/k1", "", "GET /checkout-sessions/k1"},
		{"POST", "/checkout/k1:confirm", `{"paymentMethodToken":"tok_1"}`, "POST /checkout-sessions/k1:confirm"},
		{"GET", "/orders", "", "GET /orders"},
		{"GET", "/orders/o1", "", "GET /orders/o1"},
		{"POST", "/orders/o1:cancel", `{"reason":"changed my mind"}`, "POST /orders/o1:cancel"},
		{"POST", "/shoppers", `{"email":"a@example.test"}`, "POST /shoppers"},
		{"POST", "/sessions", `{"email":"a@example.test"}`, "GET /shoppers"},
		{"GET", "/me", "", "GET /shoppers/s1"},
		{"POST", "/me/addresses", `{"line1":"1 Test Way"}`, "POST /shoppers/s1/addresses"},
	}
	for _, c := range cases {
		g := newRig()
		for _, prefix := range []string{"", BasePath} {
			rec := g.do(c.method, prefix+c.target, c.body, stateCookie)
			if rec.Code != 200 {
				t.Errorf("%s %s%s -> %d %s", c.method, prefix, c.target, rec.Code, rec.Body)
				continue
			}
			got := g.up.calls[0]
			if got.Method+" "+got.Path != c.wantCall {
				t.Errorf("%s %s -> upstream %s %s, want %s", c.method, c.target, got.Method, got.Path, c.wantCall)
			}
			g.up.calls = nil
		}
	}
}

func TestCommandsForwardMintedIdempotencyKey(t *testing.T) {
	g := newRig()
	g.do("POST", "/carts/c1/lines", `{"catalogItemId":"i1","quantity":1}`, stateCookie)
	first := g.up.last().IdempotencyKey
	g.do("POST", "/carts/c1/lines", `{"catalogItemId":"i1","quantity":1}`, stateCookie)
	if first == "" || first != g.up.last().IdempotencyKey {
		t.Fatalf("a retry must carry the same key: %q vs %q", first, g.up.last().IdempotencyKey)
	}
	g.do("POST", "/carts/c1/lines", `{"catalogItemId":"i2","quantity":1}`, stateCookie)
	if g.up.last().IdempotencyKey == first {
		t.Fatal("a different payload must not share the key")
	}
}

func TestActionHeaderPinsKey(t *testing.T) {
	g := newRig()
	do := func(body string) string {
		req := g.reqWithAction("POST", "/carts/c1/lines", body, "click-1")
		g.h.ServeHTTP(newRecorder(), req)
		return g.up.last().IdempotencyKey
	}
	if do(`{"a":1}`) != do(`{"a":2}`) {
		t.Fatal("the X-Action-Id must pin the key")
	}
}
