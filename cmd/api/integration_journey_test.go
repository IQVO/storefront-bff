//go:build integration

package main

import (
	"strings"
	"testing"
)

func upstreams(t *testing.T) map[string]*fake {
	return map[string]*fake{
		"PRODUCT_DISCOVERY_URL": newFake(t, map[string]answer{
			"GET /products/p1": {200, `{"catalogItemId":"p1","title":"Desk lamp"}`},
			"GET /products":    {200, `{"items":[{"catalogItemId":"p1"}]}`},
		}),
		"DELIVERY_PROMISE_URL": newFake(t, map[string]answer{
			"POST /delivery-offers:quote": {200, `{"offers":[{"state":"QUOTED","tier":"NEXT_DAY","window":{"earliest":"2026-10-27T08:00:00Z","latest":"2026-10-27T20:00:00Z"},"expiresAt":"2026-10-26T13:00:00Z"}],"unavailable":[]}`},
		}),
		"PRICING_URL": newFake(t, map[string]answer{
			"GET /delivery-charge-schedules/current": {200, `{"tiers":[{"tier":"NEXT_DAY","charge":{"amountMinor":599,"currency":"USD"}}]}`},
		}),
		"SHOPPING_CART_URL": newFake(t, map[string]answer{
			"POST /carts":                          {201, `{"cartId":"c1","state":"OPEN","lines":[]}`},
			"POST /carts/c1/lines":                 {200, `{"cartId":"c1","lines":[{"catalogItemId":"p1","quantity":1}]}`},
			"POST /carts/c1/lines/p1:set-quantity": {200, `{"cartId":"c1"}`},
			"POST /carts/c1:set-destination":       {200, `{"cartId":"c1"}`},
			"POST /carts/c1:quote-delivery":        {200, `{"cartId":"c1","quote":{}}`},
			"POST /carts/c1:select-delivery":       {200, `{"cartId":"c1"}`},
		}),
		"CHECKOUT_URL": newFake(t, map[string]answer{
			"POST /checkout-sessions":            {201, `{"sessionId":"k1","state":"OPEN"}`},
			"POST /checkout-sessions/k1:confirm": {200, `{"sessionId":"k1","state":"COMPLETED","orderId":"o1","currency":"USD","totalMinor":2598,"groups":[{"groupNo":1,"tier":"NEXT_DAY","amountMinor":599}]}`},
			"GET /checkout-sessions/k1":          {200, `{"sessionId":"k1","state":"COMPLETED","orderId":"o1","currency":"USD","totalMinor":2598,"groups":[{"groupNo":1}]}`},
		}),
		"CUSTOMER_ORDERING_URL": newFake(t, map[string]answer{
			"POST /shoppers":              {201, `{"shopperId":"s1","email":"a@example.test","savedAddresses":[]}`},
			"POST /shoppers/s1/addresses": {201, `{"addressId":"a1"}`},
			"GET /orders/o1":              {200, `{"orderId":"o1","shopperId":"s1","status":"PROMISED"}`},
			"GET /orders/o1/tracking":     {200, `{"orderId":"o1","groups":[]}`},
		}),
	}
}

func TestIntegrationShopperJourney(t *testing.T) {
	ups := upstreams(t)
	s := &session{t: t, base: bff(t, ups, nil)}
	steps := []struct {
		method, path, body string
		want               int
	}{
		{"GET", "/products?q=lamp", "", 200},
		{"GET", "/products/p1", "", 200},
		{"POST", "/products/p1:delivery-dates", `{"postalPrefix":"787"}`, 200},
		{"POST", "/shoppers", `{"email":"a@example.test"}`, 201},
		{"POST", "/me/addresses", `{"line1":"1 Test Way","postalCode":"78701"}`, 201},
		{"POST", "/carts", ``, 201},
		{"POST", "/carts/c1/lines", `{"catalogItemId":"p1","quantity":1}`, 200},
		{"PATCH", "/carts/c1/lines/p1", `{"quantity":2}`, 200},
		{"POST", "/carts/c1/destination", `{"destinationPostalPrefix":"787"}`, 200},
		{"POST", "/carts/c1/delivery:quote", ``, 200},
		{"POST", "/carts/c1/delivery:select", `{"groupNo":1,"offerId":"off1"}`, 200},
		{"POST", "/checkout", `{"shipToAddressId":"a1"}`, 201},
		{"POST", "/checkout/k1:confirm", `{"paymentMethodToken":"tok_sim_1"}`, 200},
	}
	for _, st := range steps {
		if code, body := s.do(st.method, st.path, st.body); code != st.want {
			t.Fatalf("%s %s = %d, want %d: %s", st.method, st.path, code, st.want, body)
		}
	}
	code, body := s.do("GET", "/orders/o1", "")
	if code != 200 || !strings.Contains(body, `"tracking"`) || !strings.Contains(body, `"totalMinor":2598`) || !strings.Contains(body, `"orderId":"o1"`) {
		t.Fatalf("joined order = %d %s", code, body)
	}
}
