package usecases

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

// RegisterShopper relays POST /shoppers. The body (e-mail, optional saved
// addresses) passes through untouched: the BFF neither stores nor logs it.
func (g *Gateway) RegisterShopper(ctx context.Context, a Actor, body []byte) (Outcome, error) {
	out, err := g.relay(ctx, composition.Command, a, orderingCall("POST", "/shoppers", body))
	return g.adoptShopper(a, out, err)
}

type loginRequest struct {
	Email string `json:"email"`
}

const maxEmailLen = 254

// Login looks a shopper up by e-mail (no password, no authentication) and
// remembers the shopper id in the state.
func (g *Gateway) Login(ctx context.Context, a Actor, body []byte) (Outcome, error) {
	var req loginRequest
	if decodeStrict(body, &req) != nil || req.Email == "" || len(req.Email) > maxEmailLen {
		return Outcome{}, composition.Invalid("body must be {email}")
	}
	c := orderingCall("GET", "/shoppers", nil)
	c.Query = url.Values{"email": {req.Email}}
	out, err := g.relay(ctx, composition.Query, a, c)
	return g.adoptShopper(a, out, err)
}

// adoptShopper stores the shopper id of a successful shopper reply.
func (g *Gateway) adoptShopper(a Actor, out Outcome, err error) (Outcome, error) {
	if err != nil || !out.Reply.OK() {
		return out, err
	}
	var s struct {
		ShopperID string `json:"shopperId"`
	}
	if json.Unmarshal(out.Reply.Body, &s) == nil && composition.ValidID(s.ShopperID) {
		out.State, out.SetState = a.State, true
		out.State.ShopperID = s.ShopperID
	}
	return out, nil
}

// Me relays GET /shoppers/{id} for the cookie's shopper.
func (g *Gateway) Me(ctx context.Context, a Actor) (Outcome, error) {
	if a.State.ShopperID == "" {
		return Outcome{}, composition.ErrNoSession
	}
	return g.relay(ctx, composition.Query, a, orderingCall("GET", "/shoppers/"+a.State.ShopperID, nil))
}

// AddAddress relays POST /me/addresses to the shopper's saved addresses. The
// address passes through; it is not stored or logged here.
func (g *Gateway) AddAddress(ctx context.Context, a Actor, body []byte) (Outcome, error) {
	if a.State.ShopperID == "" {
		return Outcome{}, composition.ErrNoSession
	}
	return g.relay(ctx, composition.Command, a, orderingCall("POST", "/shoppers/"+a.State.ShopperID+"/addresses", body))
}
