package usecases

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

func orderingCall(method, path string, body []byte) composition.Call {
	return composition.Call{Service: composition.CustomerOrdering, Method: method, Path: path, Body: body}
}

// ListOrders relays GET /orders; the shopper defaults to the cookie's.
func (g *Gateway) ListOrders(ctx context.Context, a Actor, q url.Values) (Outcome, error) {
	shopper := q.Get("shopperId")
	if shopper == "" {
		shopper = a.State.ShopperID
	}
	if !composition.ValidID(shopper) {
		return Outcome{}, composition.ErrNoSession
	}
	fwd := url.Values{"shopperId": {shopper}}
	for _, k := range []string{"after", "limit"} {
		if v := q.Get(k); v != "" {
			fwd.Set(k, v)
		}
	}
	c := orderingCall("GET", "/orders", nil)
	c.Query = fwd
	return g.relay(ctx, composition.Query, a, c)
}

// GetOrder returns the order joined with its tracking and, when the cookie's
// last checkout produced this order, the checkout totals. The order is
// required; tracking and totals degrade to absent.
func (g *Gateway) GetOrder(ctx context.Context, a Actor, id string) (Outcome, error) {
	if !composition.ValidID(id) {
		return Outcome{}, composition.Invalid("order id is malformed")
	}
	ctx, cancel := g.begin(ctx, composition.Query)
	defer cancel()
	order, err := g.call(ctx, a, orderingCall("GET", "/orders/"+id, nil))
	if err != nil || !order.OK() {
		return Outcome{Reply: order}, err
	}
	doc := map[string]json.RawMessage{}
	if json.Unmarshal(order.Body, &doc) != nil {
		return Outcome{}, composition.UpstreamError(composition.ErrUpstreamUnavailable, composition.CustomerOrdering)
	}
	if tr, err := g.call(ctx, a, orderingCall("GET", "/orders/"+id+"/tracking", nil)); err == nil && tr.OK() && json.Valid(tr.Body) {
		doc["tracking"] = tr.Body
	}
	if totals := g.checkoutTotals(ctx, a, id); totals != nil {
		doc["totals"] = totals
	}
	body, err := json.Marshal(doc)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Reply: jsonReply(200, body)}, nil
}

// checkoutTotals reads the cookie's checkout session and returns its totals
// when it placed this order.
func (g *Gateway) checkoutTotals(ctx context.Context, a Actor, orderID string) json.RawMessage {
	if a.State.CheckoutID == "" {
		return nil
	}
	r, err := g.call(ctx, a, checkoutCall("GET", "/checkout-sessions/"+a.State.CheckoutID, nil))
	if err != nil || !r.OK() {
		return nil
	}
	var s struct {
		SessionID  string            `json:"sessionId"`
		OrderID    string            `json:"orderId"`
		Currency   string            `json:"currency"`
		TotalMinor int64             `json:"totalMinor"`
		Groups     []json.RawMessage `json:"groups"`
	}
	if json.Unmarshal(r.Body, &s) != nil || s.OrderID != orderID {
		return nil
	}
	if s.Groups == nil {
		s.Groups = []json.RawMessage{}
	}
	out, err := json.Marshal(map[string]any{
		"checkoutId": s.SessionID, "currency": s.Currency, "totalMinor": s.TotalMinor, "groups": s.Groups,
	})
	if err != nil {
		return nil
	}
	return out
}

type cancelRequest struct {
	Reason string `json:"reason"`
}

const maxReasonLen = 200

// CancelOrder relays POST /orders/{id}:cancel.
func (g *Gateway) CancelOrder(ctx context.Context, a Actor, id string, body []byte) (Outcome, error) {
	var req cancelRequest
	if !composition.ValidID(id) || decodeStrict(body, &req) != nil || req.Reason == "" || len(req.Reason) > maxReasonLen {
		return Outcome{}, composition.Invalid("order id must be valid and body must be {reason} of at most 200 characters")
	}
	return g.relay(ctx, composition.Command, a, orderingCall("POST", "/orders/"+id+":cancel", body))
}
