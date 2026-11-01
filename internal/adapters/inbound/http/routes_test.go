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
		{"POST", "/products/p1/delivery-date-quotes", `{"postalPrefix":"787"}`, "POST /delivery-quotes"},
		{"POST", "/carts", `{}`, "POST /carts"},
		{"GET", "/carts?state=OPEN", "", "GET /carts"},
		{"GET", "/carts/c1", "", "GET /carts/c1"},
		{"POST", "/carts/c1/lines", `{"catalogItemId":"i1","quantity":1}`, "POST /carts/c1/lines"},
		{"PATCH", "/carts/c1/lines/i1", `{"quantity":2}`, "PUT /carts/c1/lines/i1"},
		{"DELETE", "/carts/c1/lines/i1", "", "DELETE /carts/c1/lines/i1"},
		{"PUT", "/carts/c1/destination", `{"destinationPostalPrefix":"787"}`, "PUT /carts/c1/destination"},
		{"POST", "/carts/c1/delivery-quotes", "", "POST /carts/c1/delivery-quotes"},
		{"PUT", "/carts/c1/delivery-selection", `{"groupNo":1,"offerId":"o1"}`, "PUT /carts/c1/delivery-selection"},
		{"POST", "/checkout", `{"shipToAddressId":"a1"}`, "POST /checkout-sessions"},
		{"GET", "/checkout/k1", "", "GET /checkout-sessions/k1"},
		{"POST", "/checkout/k1/confirmations", `{"paymentMethodToken":"tok_1"}`, "POST /checkout-sessions/k1/confirmations"},
		{"GET", "/orders", "", "GET /orders"},
		{"GET", "/orders/o1", "", "GET /orders/o1"},
		{"POST", "/orders/o1/cancellations", `{"reason":"changed my mind"}`, "POST /orders/o1/cancellations"},
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
	send := func(body, key string) string {
		g.h.ServeHTTP(newRecorder(), g.reqWithKey("POST", "/carts/c1/lines", body, key))
		return g.up.last().IdempotencyKey
	}
	first := send(`{"catalogItemId":"i1","quantity":1}`, "click-1")
	if first == "" || first == "click-1" || first != send(`{"catalogItemId":"i1","quantity":1}`, "click-1") {
		t.Fatalf("a retry with the same page key must carry the same minted upstream key: %q", first)
	}
	// Same page key, different body: the same upstream key, so the provider
	// sees the fingerprint mismatch and answers 409 idempotency-key-reused.
	if send(`{"catalogItemId":"i2","quantity":1}`, "click-1") != first {
		t.Fatal("the page key, not the body, owns the upstream key")
	}
	if send(`{"catalogItemId":"i1","quantity":1}`, "click-2") == first {
		t.Fatal("a fresh page key must mint a fresh upstream key")
	}
}
