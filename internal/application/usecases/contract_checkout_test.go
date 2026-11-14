// Consumer contract tests for the checkout surface the BFF composes, against
// the byte-pinned testdata/checkout.openapi.yaml. R1 routes, R2 fixtures,
// R3 real client and gateway flows (the totals join reads the same pinned
// shape), R4 the wire fields the session decodes read.
package usecases

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

// checkoutCallSites is the closed surface the BFF calls on checkout.
var checkoutCallSites = []struct{ method, path string }{
	{http.MethodPost, "/checkout-sessions"},
	{http.MethodGet, "/checkout-sessions/{sessionId}"},
	{http.MethodPost, "/checkout-sessions/{sessionId}/confirmations"},
}

// R1: the pinned spec declares every (method, path) the BFF calls.
func TestCheckoutPinnedSpecCoversEveryCallSite(t *testing.T) {
	doc := loadPinnedSpec(t, "checkout.openapi.yaml")
	for _, c := range checkoutCallSites {
		checkRouteDeclared(t, doc, c.method, c.path)
	}
}

// R2: every fixture satisfies the pinned schema of the message it stands for.
func TestCheckoutExamplesSatisfyPinnedSpec(t *testing.T) {
	doc := loadPinnedSpec(t, "checkout.openapi.yaml")
	checkExampleSatisfiesSchema(t, doc, "checkout.session.example.json", "CheckoutSession")
	checkExampleSatisfiesSchema(t, doc, "checkout.session-completed.example.json", "CheckoutSession")
	checkExampleSatisfiesSchema(t, doc, "checkout.open-request.example.json", "OpenSessionRequest")
	checkExampleSatisfiesSchema(t, doc, "checkout.confirm-request.example.json", "ConfirmSessionRequest")
}

// R3: OpenCheckout composes the upstream OpenSessionRequest (cart and shopper
// default to the cookie's), the sent body satisfies the producer's schema,
// and the sessionId of the producer's shape is adopted into the state.
func TestOpenCheckoutFlowsThroughTheRealClient(t *testing.T) {
	doc := loadPinnedSpec(t, "checkout.openapi.yaml")
	open := requestBodySchema(t, doc, http.MethodPost, "/checkout-sessions")
	g, log := newUpstreamGateway(t, composition.DefaultBudgets(), func(c loggedCall) composition.Reply {
		if c.Method == http.MethodPost && c.Path == "/checkout-sessions" {
			return jsonFixtureReply(t, http.StatusCreated, "checkout.session.example.json")
		}
		return composition.Reply{}
	})
	actor := Actor{
		State:          composition.State{CartID: "cart-7701", ShopperID: "shopper-9"},
		IdempotencyKey: "page-1",
	}
	out, err := g.OpenCheckout(t.Context(), actor, []byte(`{"shipToAddressId":"addr-3001"}`))
	if err != nil {
		t.Fatal(err)
	}
	calls := log.to(composition.Checkout)
	if len(calls) != 1 {
		t.Fatalf("calls = %+v, want one POST /checkout-sessions", calls)
	}
	var sent map[string]string
	if err := json.Unmarshal([]byte(calls[0].Body), &sent); err != nil {
		t.Fatalf("sent body: %v", err)
	}
	if sent["cartId"] != "cart-7701" || sent["shopperId"] != "shopper-9" || sent["shipToAddressId"] != "addr-3001" {
		t.Errorf("sent = %v, want the cookie defaults and the address id", sent)
	}
	checkBodySatisfiesSchema(t, open, []byte(calls[0].Body), "the open-checkout request")
	if !out.SetState || out.State.CheckoutID != "cs-5001" {
		t.Errorf("state = %+v (SetState %v), want cs-5001 adopted", out.State, out.SetState)
	}
}

// R3: ConfirmCheckout relays the PSP token the producer's ConfirmSessionRequest
// describes; a card-shaped body never leaves the edge.
func TestConfirmCheckoutFlowsThroughTheRealClient(t *testing.T) {
	doc := loadPinnedSpec(t, "checkout.openapi.yaml")
	confirm := requestBodySchema(t, doc, http.MethodPost, "/checkout-sessions/{sessionId}/confirmations")
	g, log := newUpstreamGateway(t, composition.DefaultBudgets(), func(c loggedCall) composition.Reply {
		return jsonFixtureReply(t, http.StatusOK, "checkout.session-completed.example.json")
	})
	body := readContractExample(t, "checkout.confirm-request.example.json")
	out, err := g.ConfirmCheckout(t.Context(), keyed, "cs-5001", body)
	if err != nil {
		t.Fatal(err)
	}
	calls := log.to(composition.Checkout)
	if len(calls) != 1 || calls[0].Method != http.MethodPost || calls[0].Path != "/checkout-sessions/cs-5001/confirmations" {
		t.Fatalf("calls = %+v, want one POST confirmations", calls)
	}
	checkBodySatisfiesSchema(t, confirm, []byte(calls[0].Body), "the confirm request")
	if calls[0].IdempotencyKey == "" {
		t.Error("a keyed page submit must carry a minted Idempotency-Key")
	}
	if out.Reply.Status != http.StatusOK {
		t.Errorf("reply status = %d, want the producer answer relayed", out.Reply.Status)
	}
}

// R4: every wire field the session decodes read (the sessionId OpenCheckout
// adopts, the totals GetOrder joins) is declared by the pinned schema.
// sessionAdoptFixture mirrors the anonymous {sessionId} decode in
// checkout.go; sessionTotalsFixture mirrors the checkoutTotals decode in
// orders.go, both verbatim.
func TestCheckoutFieldsWeReadAreSpecified(t *testing.T) {
	doc := loadPinnedSpec(t, "checkout.openapi.yaml")
	checkDTOFieldsOfSchema(t, doc, "CheckoutSession", reflect.TypeOf(sessionAdoptFixture{}), true)
	checkDTOFieldsOfSchema(t, doc, "CheckoutSession", reflect.TypeOf(sessionTotalsFixture{}), true)
}

// sessionAdoptFixture mirrors the anonymous {sessionId} decode in checkout.go.
type sessionAdoptFixture struct {
	SessionID string `json:"sessionId"`
}

// sessionTotalsFixture mirrors the checkoutTotals decode in orders.go.
type sessionTotalsFixture struct {
	SessionID  string            `json:"sessionId"`
	OrderID    string            `json:"orderId"`
	Currency   string            `json:"currency"`
	TotalMinor int64             `json:"totalMinor"`
	Groups     []json.RawMessage `json:"groups"`
}
