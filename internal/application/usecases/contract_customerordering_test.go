// Consumer contract tests for the customer-ordering surface the BFF
// composes, against the byte-pinned testdata/customer-ordering.openapi.yaml.
// R1 routes, R2 fixtures, R3 real client and gateway flows (register/login
// adoption, the order+tracking+checkout-totals join), R4 the wire fields the
// decodes read.
package usecases

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

// orderingCallSites is the closed surface the BFF calls on customer-ordering.
var orderingCallSites = []struct{ method, path string }{
	{http.MethodGet, "/orders"},
	{http.MethodGet, "/orders/{orderId}"},
	{http.MethodGet, "/orders/{orderId}/tracking"},
	{http.MethodPost, "/orders/{orderId}/cancellations"},
	{http.MethodGet, "/shoppers"},
	{http.MethodPost, "/shoppers"},
	{http.MethodGet, "/shoppers/{shopperId}"},
	{http.MethodPost, "/shoppers/{shopperId}/addresses"},
}

// R1: the pinned spec declares every (method, path) the BFF calls.
func TestOrderingPinnedSpecCoversEveryCallSite(t *testing.T) {
	doc := loadPinnedSpec(t, "customer-ordering.openapi.yaml")
	for _, c := range orderingCallSites {
		checkRouteDeclared(t, doc, c.method, c.path)
	}
}

// R2: every fixture satisfies the pinned schema of the message it stands for.
func TestOrderingExamplesSatisfyPinnedSpec(t *testing.T) {
	doc := loadPinnedSpec(t, "customer-ordering.openapi.yaml")
	checkExampleSatisfiesSchema(t, doc, "customer-ordering.shopper.example.json", "Shopper")
	checkExampleSatisfiesSchema(t, doc, "customer-ordering.orders.example.json", "CustomerOrderList")
	checkExampleSatisfiesSchema(t, doc, "customer-ordering.order.example.json", "CustomerOrder")
	checkExampleSatisfiesSchema(t, doc, "customer-ordering.tracking.example.json", "OrderTracking")
	checkExampleSatisfiesSchema(t, doc, "customer-ordering.register-request.example.json", "RegisterShopperRequest")
	checkExampleSatisfiesSchema(t, doc, "customer-ordering.cancel-request.example.json", "CancelOrderRequest")
}

// R3: RegisterShopper relays the page's body byte-for-byte (it satisfies the
// producer's RegisterShopperRequest) and adopts the minted shopperId.
func TestRegisterShopperFlowsThroughTheRealClient(t *testing.T) {
	doc := loadPinnedSpec(t, "customer-ordering.openapi.yaml")
	register := requestBodySchema(t, doc, http.MethodPost, "/shoppers")
	g, log := newUpstreamGateway(t, composition.DefaultBudgets(), func(c loggedCall) composition.Reply {
		if c.Method == http.MethodPost && c.Path == "/shoppers" {
			return jsonFixtureReply(t, http.StatusCreated, "customer-ordering.shopper.example.json")
		}
		return composition.Reply{}
	})
	body := readContractExample(t, "customer-ordering.register-request.example.json")
	out, err := g.RegisterShopper(t.Context(), keyed, body)
	if err != nil {
		t.Fatal(err)
	}
	calls := log.to(composition.CustomerOrdering)
	if len(calls) != 1 || calls[0].Method != http.MethodPost || calls[0].Path != "/shoppers" {
		t.Fatalf("calls = %+v, want one POST /shoppers", calls)
	}
	if calls[0].Body != string(body) {
		t.Error("the registration body must pass through untouched")
	}
	checkBodySatisfiesSchema(t, register, []byte(calls[0].Body), "the register request")
	if !out.SetState || out.State.ShopperID != "shopper-9" {
		t.Errorf("state = %+v (SetState %v), want shopper-9 adopted", out.State, out.SetState)
	}
}

// R3: Login looks the shopper up by e-mail on the producer's query contract
// and adopts the shopper the producer's Shopper shape carries.
func TestLoginFlowsThroughTheRealClient(t *testing.T) {
	g, log := newUpstreamGateway(t, composition.DefaultBudgets(), func(c loggedCall) composition.Reply {
		if c.Method == http.MethodGet && c.Path == "/shoppers" {
			return jsonFixtureReply(t, http.StatusOK, "customer-ordering.shopper.example.json")
		}
		return composition.Reply{}
	})
	out, err := g.Login(t.Context(), keyed, []byte(`{"email":"ada@example.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	calls := log.to(composition.CustomerOrdering)
	if len(calls) != 1 {
		t.Fatalf("calls = %+v, want one GET /shoppers", calls)
	}
	if got := parseQuery(calls[0].RawQuery).Get("email"); got != "ada@example.com" {
		t.Errorf("email = %q, want the lookup key forwarded", got)
	}
	if !out.SetState || out.State.ShopperID != "shopper-9" {
		t.Errorf("state = %+v (SetState %v), want shopper-9 adopted", out.State, out.SetState)
	}
}

// orderingAndCheckoutReply answers GetOrder's three reads: the order, its
// tracking and the cookie's completed checkout session.
func orderingAndCheckoutReply(t *testing.T) func(loggedCall) composition.Reply {
	t.Helper()
	return func(c loggedCall) composition.Reply {
		switch c.Service {
		case "customer-ordering":
			switch {
			case c.Method == http.MethodGet && c.Path == "/orders/ord-5001":
				return jsonFixtureReply(t, http.StatusOK, "customer-ordering.order.example.json")
			case c.Method == http.MethodGet && c.Path == "/orders/ord-5001/tracking":
				return jsonFixtureReply(t, http.StatusOK, "customer-ordering.tracking.example.json")
			}
		case "checkout":
			return jsonFixtureReply(t, http.StatusOK, "checkout.session-completed.example.json")
		}
		return composition.Reply{}
	}
}

// R3: GetOrder composes the producer's CustomerOrder with its OrderTracking
// and, when the cookie's checkout placed it, the checkout session's totals —
// three pinned shapes (two producers) through the REAL client.
func TestGetOrderComposesThroughTheRealClient(t *testing.T) {
	g, log := newUpstreamGateway(t, composition.DefaultBudgets(), orderingAndCheckoutReply(t))
	actor := Actor{State: composition.State{CheckoutID: "cs-5001"}}
	out, err := g.GetOrder(t.Context(), actor, "ord-5001")
	if err != nil {
		t.Fatal(err)
	}
	if out.Reply.Status != http.StatusOK {
		t.Fatalf("status = %d", out.Reply.Status)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(out.Reply.Body, &doc); err != nil {
		t.Fatal(err)
	}
	for _, joined := range []string{"orderId", "tracking", "totals"} {
		if _, ok := doc[joined]; !ok {
			t.Errorf("composed order lacks %q: %s", joined, out.Reply.Body)
		}
	}
	var totals struct {
		CheckoutID string `json:"checkoutId"`
		TotalMinor int64  `json:"totalMinor"`
	}
	if err := json.Unmarshal(doc["totals"], &totals); err != nil {
		t.Fatal(err)
	}
	if totals.CheckoutID != "cs-5001" || totals.TotalMinor != 2758 {
		t.Errorf("totals = %+v, want cs-5001/2758 from the pinned session shape", totals)
	}
	if len(log.to(composition.CustomerOrdering)) != 2 || len(log.to(composition.Checkout)) != 1 {
		t.Errorf("calls: ordering %d checkout %d, want 2 and 1", len(log.to(composition.CustomerOrdering)), len(log.to(composition.Checkout)))
	}
}

// R4: every wire field the decodes read is declared by the pinned schema —
// the named cancelRequest the edge validates byte-equals the producer's
// CancelOrderRequest, and shopperAdoptFixture mirrors the anonymous
// {shopperId} decode in shoppers.go verbatim.
func TestOrderingFieldsWeReadAreSpecified(t *testing.T) {
	doc := loadPinnedSpec(t, "customer-ordering.openapi.yaml")
	checkDTOFieldsOfSchema(t, doc, "CancelOrderRequest", reflect.TypeOf(cancelRequest{}), false)
	checkDTOFieldsOfSchema(t, doc, "Shopper", reflect.TypeOf(shopperAdoptFixture{}), true)
}

// shopperAdoptFixture mirrors the anonymous {shopperId} decode in shoppers.go.
type shopperAdoptFixture struct {
	ShopperID string `json:"shopperId"`
}
