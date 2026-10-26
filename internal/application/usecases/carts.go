package usecases

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

func cartCall(method, path string, body []byte) composition.Call {
	return composition.Call{Service: composition.ShoppingCart, Method: method, Path: path, Body: body}
}

// CreateCart relays POST /carts and remembers the new cart id in the state.
// An empty body adopts the cookie's shopper, so a signed-in cart is owned.
func (g *Gateway) CreateCart(ctx context.Context, a Actor, body []byte) (Outcome, error) {
	if len(body) == 0 {
		body = []byte(`{}`)
		if a.State.ShopperID != "" {
			body, _ = json.Marshal(map[string]string{"shopperId": a.State.ShopperID})
		}
	}
	out, err := g.relay(ctx, composition.Command, a, cartCall("POST", "/carts", body))
	if err != nil || !out.Reply.OK() {
		return out, err
	}
	var cart struct {
		CartID string `json:"cartId"`
	}
	if json.Unmarshal(out.Reply.Body, &cart) == nil && composition.ValidID(cart.CartID) {
		out.State, out.SetState = a.State, true
		out.State.CartID = cart.CartID
	}
	return out, nil
}

// GetCart relays GET /carts/{id}.
func (g *Gateway) GetCart(ctx context.Context, id string) (Outcome, error) {
	if !composition.ValidID(id) {
		return Outcome{}, composition.Invalid("cart id is malformed")
	}
	return g.relay(ctx, composition.Query, Actor{}, cartCall("GET", "/carts/"+id, nil))
}

// ListCarts relays GET /carts?shopperId=&state=; the shopper defaults to the
// cookie's.
func (g *Gateway) ListCarts(ctx context.Context, a Actor, q url.Values) (Outcome, error) {
	shopper := q.Get("shopperId")
	if shopper == "" {
		shopper = a.State.ShopperID
	}
	if !composition.ValidID(shopper) {
		return Outcome{}, composition.Invalid("shopperId is required")
	}
	fwd := url.Values{"shopperId": {shopper}}
	if s := q.Get("state"); s != "" {
		fwd.Set("state", s)
	}
	c := cartCall("GET", "/carts", nil)
	c.Query = fwd
	return g.relay(ctx, composition.Query, a, c)
}

// AddLine relays POST /carts/{id}/lines.
func (g *Gateway) AddLine(ctx context.Context, a Actor, cartID string, body []byte) (Outcome, error) {
	return g.cartCommand(ctx, a, cartID, "/lines", body)
}

// SetDestination relays POST /carts/{id}/destination.
func (g *Gateway) SetDestination(ctx context.Context, a Actor, cartID string, body []byte) (Outcome, error) {
	return g.cartCommand(ctx, a, cartID, ":set-destination", body)
}

// QuoteDelivery relays POST /carts/{id}/delivery:quote. The cart service calls
// delivery-promise synchronously, so it gets the command budget.
func (g *Gateway) QuoteDelivery(ctx context.Context, a Actor, cartID string) (Outcome, error) {
	return g.cartCommand(ctx, a, cartID, ":quote-delivery", nil)
}

// SelectDelivery relays POST /carts/{id}/delivery:select.
func (g *Gateway) SelectDelivery(ctx context.Context, a Actor, cartID string, body []byte) (Outcome, error) {
	return g.cartCommand(ctx, a, cartID, ":select-delivery", body)
}

// SetLineQuantity relays PATCH /carts/{id}/lines/{item} as the cart's
// set-quantity command.
func (g *Gateway) SetLineQuantity(ctx context.Context, a Actor, cartID, itemID string, body []byte) (Outcome, error) {
	return g.cartCommand(ctx, a, cartID, "/lines/"+itemID+":set-quantity", body, itemID)
}

// RemoveLine relays DELETE /carts/{id}/lines/{item} as the cart's remove
// command.
func (g *Gateway) RemoveLine(ctx context.Context, a Actor, cartID, itemID string) (Outcome, error) {
	return g.cartCommand(ctx, a, cartID, "/lines/"+itemID+":remove", nil, itemID)
}

func (g *Gateway) cartCommand(ctx context.Context, a Actor, cartID, suffix string, body []byte, extra ...string) (Outcome, error) {
	for _, id := range append([]string{cartID}, extra...) {
		if !composition.ValidID(id) {
			return Outcome{}, composition.Invalid("an id in the path is malformed")
		}
	}
	return g.relay(ctx, composition.Command, a, cartCall("POST", "/carts/"+cartID+suffix, body))
}
