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
		s.doWithKey("POST", "/carts/c1/lines", `{"catalogItemId":"p1","quantity":1}`, "submit-1")
	}
	s.do("POST", "/checkout", `{"shipToAddressId":"a1"}`)
	s.do("POST", "/products/p1/delivery-date-quotes", `{"postalPrefix":"787"}`)

	cart := ups["SHOPPING_CART_URL"]
	reqs := cart.requests()
	if len(reqs) < 3 || reqs[1].Key == "" || reqs[1].Key != reqs[2].Key {
		t.Fatalf("a retried command must carry one minted key: %+v", reqs)
	}
	co, ok := ups["CHECKOUT_URL"].find("POST", "/checkout-sessions")
	if !ok || co.Key == "" || co.Body != `{"cartId":"c1","shipToAddressId":"a1","shopperId":"s1"}` {
		t.Fatalf("composed open-session request = %+v", co)
	}
	q, _ := ups["DELIVERY_PROMISE_URL"].find("POST", "/delivery-quotes")
	if q.Body != `{"destinationPostalPrefix":"787","lines":[{"catalogItemId":"p1","quantity":1}]}` {
		t.Fatalf("one-line quote = %s", q.Body)
	}
}

// A listing page's batch estimate is a safe POST: the real binary relays the
// producer's answer unchanged (capacityBand included), sends the producer the
// destination and the items only, and forwards no Idempotency-Key, whether the
// page sent one or not.
func TestIntegrationDeliveryEstimatesAreRelayedWithoutAKey(t *testing.T) {
	ups := upstreams(t)
	s := &session{t: t, base: bff(t, ups, nil)}
	const req = `{"postalPrefix":"787","items":[{"catalogItemId":"p1","quantity":1}]}`
	for _, key := range []string{"", "page-key"} {
		code, body := s.doWithKey("POST", "/delivery-estimates", req, key)
		if code != 200 || !strings.Contains(body, `"capacityBand":"TIGHT"`) || !strings.Contains(body, `"start":"2026-10-27T08:00:00Z"`) {
			t.Fatalf("key %q: estimate = %d %s", key, code, body)
		}
	}
	reqs := ups["DELIVERY_PROMISE_URL"].requests()
	if len(reqs) != 2 {
		t.Fatalf("upstream requests = %+v, want two", reqs)
	}
	for _, r := range reqs {
		if r.Method != "POST" || r.Path != "/delivery-estimates" || r.Key != "" ||
			r.Body != `{"destinationPostalPrefix":"787","items":[{"catalogItemId":"p1","quantity":1}]}` {
			t.Errorf("estimate request = %+v", r)
		}
	}
	if code, _ := s.doWithKey("POST", "/delivery-estimates", `{"postalPrefix":"787","items":[]}`, ""); code != 422 {
		t.Errorf("an empty batch = %d, want 422", code)
	}
	if n := len(ups["DELIVERY_PROMISE_URL"].requests()); n != 2 {
		t.Errorf("an invalid batch reached the producer (%d requests)", n)
	}
}

func TestIntegrationNeverCallsInOrgRoutes(t *testing.T) {
	ups := upstreams(t)
	s := &session{t: t, base: bff(t, ups, nil)}
	s.do("POST", "/shoppers", `{"email":"a@example.test"}`)
	s.do("GET", "/orders/o1", "")
	s.do("POST", "/orders/o1/cancellations", `{"reason":"changed my mind"}`)
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
		if code, _ := s.do("POST", "/checkout/k1/confirmations", body); code != 422 {
			t.Errorf("%s = %d, want 422", body, code)
		}
	}
	if _, ok := ups["CHECKOUT_URL"].find("POST", "/checkout-sessions/k1/confirmations"); ok {
		t.Fatal("card data reached checkout")
	}
}
