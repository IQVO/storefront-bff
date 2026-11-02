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
			body, _ = json.Marshal(map[string]string{shopperIDParam: a.State.ShopperID})
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
	return g.relay(ctx, composition.Query, Actor{}, cartCall(methodGet, "/carts/"+id, nil))
}

// ListCarts relays GET /carts?shopperId=&state=&limit=&cursor=; the shopper
// defaults to the cookie's. The answer is the provider's {items, nextCursor}.
func (g *Gateway) ListCarts(ctx context.Context, a Actor, q url.Values) (Outcome, error) {
	shopper := q.Get(shopperIDParam)
	if shopper == "" {
		shopper = a.State.ShopperID
	}
	if !composition.ValidID(shopper) {
		return Outcome{}, composition.Invalid("shopperId is required")
	}
	fwd := url.Values{shopperIDParam: {shopper}}
	forward(fwd, q, "state", "limit", "cursor")
	c := cartCall(methodGet, "/carts", nil)
	c.Query = fwd
	return g.relay(ctx, composition.Query, a, c)
}

// AddLine relays POST /carts/{id}/lines.
func (g *Gateway) AddLine(ctx context.Context, a Actor, cartID string, body []byte) (Outcome, error) {
	return g.cartCommand(ctx, a, "POST", cartID, "/lines", body)
}

// SetDestination relays PUT /carts/{id}/destination.
func (g *Gateway) SetDestination(ctx context.Context, a Actor, cartID string, body []byte) (Outcome, error) {
	return g.cartCommand(ctx, a, "PUT", cartID, "/destination", body)
}

// QuoteDelivery relays POST /carts/{id}/delivery-quotes. The cart service
// calls delivery-promise synchronously, so it gets the command budget.
func (g *Gateway) QuoteDelivery(ctx context.Context, a Actor, cartID string) (Outcome, error) {
	return g.cartCommand(ctx, a, "POST", cartID, "/delivery-quotes", nil)
}

// SelectDelivery relays PUT /carts/{id}/delivery-selection.
func (g *Gateway) SelectDelivery(ctx context.Context, a Actor, cartID string, body []byte) (Outcome, error) {
	return g.cartCommand(ctx, a, "PUT", cartID, "/delivery-selection", body)
}

// SetLineQuantity relays PATCH /carts/{id}/lines/{item} as the cart's
// PUT /carts/{id}/lines/{item} (replace the line quantity).
func (g *Gateway) SetLineQuantity(ctx context.Context, a Actor, cartID, itemID string, body []byte) (Outcome, error) {
	return g.cartCommand(ctx, a, "PUT", cartID, "/lines/"+itemID, body, itemID)
}

// RemoveLine relays DELETE /carts/{id}/lines/{item}.
func (g *Gateway) RemoveLine(ctx context.Context, a Actor, cartID, itemID string) (Outcome, error) {
	return g.cartCommand(ctx, a, "DELETE", cartID, "/lines/"+itemID, nil, itemID)
}

func (g *Gateway) cartCommand(ctx context.Context, a Actor, method, cartID, suffix string, body []byte, extra ...string) (Outcome, error) {
	for _, id := range append([]string{cartID}, extra...) {
		if !composition.ValidID(id) {
			return Outcome{}, composition.Invalid("an id in the path is malformed")
		}
	}
	return g.relay(ctx, composition.Command, a, cartCall(method, "/carts/"+cartID+suffix, body))
}
