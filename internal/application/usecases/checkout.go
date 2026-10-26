package usecases

import (
	"context"
	"encoding/json"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

func checkoutCall(method, path string, body []byte) composition.Call {
	return composition.Call{Service: composition.Checkout, Method: method, Path: path, Body: body}
}

type openCheckoutRequest struct {
	CartID          string `json:"cartId"`
	ShipToAddressID string `json:"shipToAddressId"`
}

// OpenCheckout relays POST /checkout as checkout's OpenSession. The cart and
// the shopper default to the cookie's; the saved address is only an id.
func (g *Gateway) OpenCheckout(ctx context.Context, a Actor, body []byte) (Outcome, error) {
	var req openCheckoutRequest
	if len(body) > 0 {
		if err := decodeStrict(body, &req); err != nil {
			return Outcome{}, composition.Invalid("body must be {cartId?, shipToAddressId}")
		}
	}
	if req.CartID == "" {
		req.CartID = a.State.CartID
	}
	if a.State.ShopperID == "" {
		return Outcome{}, composition.ErrNoSession
	}
	if !composition.ValidID(req.CartID) || !composition.ValidID(req.ShipToAddressID) {
		return Outcome{}, composition.Invalid("cartId and shipToAddressId are required ids")
	}
	open, err := json.Marshal(map[string]string{
		"cartId": req.CartID, "shopperId": a.State.ShopperID, "shipToAddressId": req.ShipToAddressID,
	})
	if err != nil {
		return Outcome{}, err
	}
	out, err := g.relay(ctx, composition.Command, a, checkoutCall("POST", "/checkout-sessions", open))
	if err != nil || !out.Reply.OK() {
		return out, err
	}
	var s struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(out.Reply.Body, &s) == nil && composition.ValidID(s.SessionID) {
		out.State, out.SetState = a.State, true
		out.State.CheckoutID = s.SessionID
	}
	return out, nil
}

// GetCheckout relays GET /checkout/{id}; the page polls it while a placement
// is in doubt (202).
func (g *Gateway) GetCheckout(ctx context.Context, id string) (Outcome, error) {
	if !composition.ValidID(id) {
		return Outcome{}, composition.Invalid("checkout id is malformed")
	}
	return g.relay(ctx, composition.Query, Actor{}, checkoutCall("GET", "/checkout-sessions/"+id, nil))
}

type confirmRequest struct {
	PaymentMethodToken string `json:"paymentMethodToken"`
}

const maxTokenLen = 256

// ConfirmCheckout relays POST /checkout/{id}:confirm with the PSP token. Raw
// card data is refused at the edge: unknown fields, card-shaped keys and a
// token that is really a card number never leave this process.
func (g *Gateway) ConfirmCheckout(ctx context.Context, a Actor, id string, body []byte) (Outcome, error) {
	if !composition.ValidID(id) {
		return Outcome{}, composition.Invalid("checkout id is malformed")
	}
	var probe map[string]json.RawMessage
	if json.Unmarshal(body, &probe) == nil {
		for k := range probe {
			if composition.IsCardFieldName(k) {
				return Outcome{}, composition.Invalid("card data is refused; send the payment method token only")
			}
		}
	}
	var req confirmRequest
	if err := decodeStrict(body, &req); err != nil {
		return Outcome{}, composition.Invalid("body must be {paymentMethodToken}")
	}
	if req.PaymentMethodToken == "" || len(req.PaymentMethodToken) > maxTokenLen || composition.LooksLikeCardNumber(req.PaymentMethodToken) {
		return Outcome{}, composition.Invalid("paymentMethodToken must be a PSP token, never a card number")
	}
	return g.relay(ctx, composition.Confirm, a, checkoutCall("POST", "/checkout-sessions/"+id+":confirm", body))
}
