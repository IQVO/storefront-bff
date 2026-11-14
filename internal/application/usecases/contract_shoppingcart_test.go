// Consumer contract tests for the shopping-cart surface the BFF composes:
// the pinned spec (testdata/shopping-cart.openapi.yaml) versus what the
// use cases really send and decode. R1 routes, R2 fixtures, R3 real client
// and gateway flows, R4 the wire fields the decodes read.
package usecases

import (
	"net/http"
	"net/url"
	"reflect"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

// parseQuery is the test-side query parser for forwarded parameters.
func parseQuery(raw string) url.Values {
	v, err := url.ParseQuery(raw)
	if err != nil {
		panic(err)
	}
	return v
}

// checkDTOFieldsOfSchema runs checkDTOFields over a named pinned schema.
func checkDTOFieldsOfSchema(t *testing.T, doc *openapi3.T, schema string, dt reflect.Type, decode bool) {
	t.Helper()
	ref := doc.Components.Schemas[schema]
	if ref == nil || ref.Value == nil {
		t.Errorf("schema %s is not in the pinned spec copy", schema)
		return
	}
	checkDTOFields(t, dt, ref.Value, schema, decode)
}

// shoppingCartCallSites is the closed surface the BFF calls on shopping-cart
// (composition's allowlist); path templates as the pinned spec spells them.
var shoppingCartCallSites = []struct{ method, path string }{
	{http.MethodPost, "/carts"},
	{http.MethodGet, "/carts"},
	{http.MethodGet, "/carts/{cartId}"},
	{http.MethodPost, "/carts/{cartId}/lines"},
	{http.MethodPut, "/carts/{cartId}/lines/{catalogItemId}"},
	{http.MethodDelete, "/carts/{cartId}/lines/{catalogItemId}"},
	{http.MethodPut, "/carts/{cartId}/destination"},
	{http.MethodPost, "/carts/{cartId}/delivery-quotes"},
	{http.MethodPut, "/carts/{cartId}/delivery-selection"},
}

// R1: the pinned spec declares every (method, path) the BFF calls.
func TestShoppingCartPinnedSpecCoversEveryCallSite(t *testing.T) {
	doc := loadPinnedSpec(t, "shopping-cart.openapi.yaml")
	for _, c := range shoppingCartCallSites {
		checkRouteDeclared(t, doc, c.method, c.path)
	}
}

// R2: every fixture satisfies the pinned schema of the message it stands for.
func TestShoppingCartExamplesSatisfyPinnedSpec(t *testing.T) {
	doc := loadPinnedSpec(t, "shopping-cart.openapi.yaml")
	checkExampleSatisfiesSchema(t, doc, "shopping-cart.cart.example.json", "Cart")
	checkExampleSatisfiesSchema(t, doc, "shopping-cart.carts.example.json", "CartPage")
	checkExampleSatisfiesSchema(t, doc, "shopping-cart.line-request.example.json", "LineInput")
}

// R3: CreateCart flows through the REAL client into the cart service and
// adopts the cart the producer's shape carries; the sent body satisfies the
// producer's inline {shopperId} request schema and a keyed page submit makes
// the client mint the Idempotency-Key the fleet guideline requires.
func TestCreateCartFlowsThroughTheRealClient(t *testing.T) {
	doc := loadPinnedSpec(t, "shopping-cart.openapi.yaml")
	create := requestBodySchema(t, doc, http.MethodPost, "/carts")
	g, log := newUpstreamGateway(t, composition.DefaultBudgets(), func(c loggedCall) composition.Reply {
		if c.Method == http.MethodPost && c.Path == "/carts" {
			return jsonFixtureReply(t, http.StatusCreated, "shopping-cart.cart.example.json")
		}
		return composition.Reply{}
	})
	actor := Actor{IdempotencyKey: "page-1"}
	out, err := g.CreateCart(t.Context(), actor, nil)
	if err != nil {
		t.Fatal(err)
	}
	calls := log.to(composition.ShoppingCart)
	if len(calls) != 1 || calls[0].Method != http.MethodPost || calls[0].Path != "/carts" {
		t.Fatalf("calls = %+v, want one POST /carts", calls)
	}
	if calls[0].IdempotencyKey == "" {
		t.Error("a keyed page submit must carry a minted Idempotency-Key")
	}
	checkBodySatisfiesSchema(t, create, []byte(calls[0].Body), "the create-cart request")
	if !out.SetState || out.State.CartID != "cart-7701" {
		t.Errorf("state = %+v (SetState %v), want cart-7701 adopted", out.State, out.SetState)
	}
	if out.Reply.Status != http.StatusCreated || out.Reply.ContentType != "application/json" {
		t.Errorf("reply = %d %s, want 201 application/json relayed", out.Reply.Status, out.Reply.ContentType)
	}
}

// R3: ListCarts forwards the shopper and the producer's paging filters and
// relays the {items, nextCursor} page unchanged.
func TestListCartsFlowsThroughTheRealClient(t *testing.T) {
	g, log := newUpstreamGateway(t, composition.DefaultBudgets(), func(c loggedCall) composition.Reply {
		if c.Method == http.MethodGet && c.Path == "/carts" {
			return jsonFixtureReply(t, http.StatusOK, "shopping-cart.carts.example.json")
		}
		return composition.Reply{}
	})
	actor := Actor{State: composition.State{ShopperID: "shopper-9"}}
	out, err := g.ListCarts(t.Context(), actor, parseQuery("state=OPEN&limit=20&cursor=c-8&dropped=yes"))
	if err != nil {
		t.Fatal(err)
	}
	calls := log.to(composition.ShoppingCart)
	if len(calls) != 1 {
		t.Fatalf("calls = %+v, want one GET /carts", calls)
	}
	q := parseQuery(calls[0].RawQuery)
	for _, k := range []string{"shopperId", "state", "limit", "cursor"} {
		if !q.Has(k) {
			t.Errorf("query %q lacks %s", calls[0].RawQuery, k)
		}
	}
	if q.Has("dropped") {
		t.Error("a parameter outside the allowlist must not be forwarded")
	}
	if out.Reply.Status != http.StatusOK || len(out.Reply.Body) == 0 {
		t.Errorf("reply = %d, want the page relayed", out.Reply.Status)
	}
}

// R4: every wire field the cart decodes read (the cart id CreateCart adopts,
// the line the page adds) is declared by the pinned schema, and optional
// fields are tolerated absent. cartAdoptFixture mirrors the anonymous
// {cartId} decode in carts.go verbatim; lineBodyFixture mirrors the LineInput
// body AddLine relays unread.
func TestShoppingCartFieldsWeReadAreSpecified(t *testing.T) {
	doc := loadPinnedSpec(t, "shopping-cart.openapi.yaml")
	checkDTOFieldsOfSchema(t, doc, "Cart", reflect.TypeOf(cartAdoptFixture{}), true)
	checkDTOFieldsOfSchema(t, doc, "LineInput", reflect.TypeOf(lineBodyFixture{}), false)
}

// cartAdoptFixture mirrors the anonymous {cartId} decode in carts.go.
type cartAdoptFixture struct {
	CartID string `json:"cartId"`
}

// lineBodyFixture mirrors the LineInput body AddLine relays unread.
type lineBodyFixture struct {
	CatalogItemID string `json:"catalogItemId"`
	Quantity      int    `json:"quantity"`
}
