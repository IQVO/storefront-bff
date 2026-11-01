//go:build integration

package main

import (
	"strings"
	"testing"
)

func TestIntegrationComposedContractAndKeys(t *testing.T) {
	ups := upstreams(t)
	s := &session{t: t, base: bff(t, ups, nil)}
	s.do("POST", "/shoppers", `{"email":"a@example.test"}`)
	s.do("POST", "/carts", ``)
	for range 2 {
		s.do("POST", "/carts/c1/lines", `{"catalogItemId":"p1","quantity":1}`)
	}
	s.do("POST", "/checkout", `{"shipToAddressId":"a1"}`)
	s.do("POST", "/products/p1:delivery-dates", `{"postalPrefix":"787"}`)

	cart := ups["SHOPPING_CART_URL"]
	reqs := cart.requests()
	if len(reqs) < 3 || reqs[1].Key == "" || reqs[1].Key != reqs[2].Key {
		t.Fatalf("a retried command must carry one minted key: %+v", reqs)
	}
	co, ok := ups["CHECKOUT_URL"].find("POST", "/checkout-sessions")
	if !ok || co.Key == "" || co.Body != `{"cartId":"c1","shipToAddressId":"a1","shopperId":"s1"}` {
		t.Fatalf("composed open-session request = %+v", co)
	}
	q, _ := ups["DELIVERY_PROMISE_URL"].find("POST", "/delivery-offers:quote")
	if q.Body != `{"destinationPostalPrefix":"787","lines":[{"catalogItemId":"p1","quantity":1}]}` {
		t.Fatalf("one-line quote = %s", q.Body)
	}
}

func TestIntegrationNeverCallsInOrgRoutes(t *testing.T) {
	ups := upstreams(t)
	s := &session{t: t, base: bff(t, ups, nil)}
	s.do("POST", "/shoppers", `{"email":"a@example.test"}`)
	s.do("GET", "/orders/o1", "")
	s.do("POST", "/orders/o1:cancel", `{"reason":"changed my mind"}`)
	for name, f := range ups {
		for _, r := range f.requests() {
			if strings.Contains(r.Path, "ship-to") || strings.Contains(r.Path, "/promises") {
				t.Errorf("%s received a restricted route %s", name, r.Path)
			}
		}
	}
}

func TestIntegrationCardDataNeverReachesCheckout(t *testing.T) {
	ups := upstreams(t)
	s := &session{t: t, base: bff(t, ups, nil)}
	for _, body := range []string{`{"paymentMethodToken":"4111111111111111"}`, `{"paymentMethodToken":"t","cvc":"123"}`} {
		if code, _ := s.do("POST", "/checkout/k1:confirm", body); code != 422 {
			t.Errorf("%s = %d, want 422", body, code)
		}
	}
	if _, ok := ups["CHECKOUT_URL"].find("POST", "/checkout-sessions/k1:confirm"); ok {
		t.Fatal("card data reached checkout")
	}
}
