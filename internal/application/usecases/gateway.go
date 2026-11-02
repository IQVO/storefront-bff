// Package usecases composes the storefront routes over the upstream services.
package usecases

import (
	"context"
	"errors"
	"net/url"

	"github.com/IQVO/storefront-bff/internal/application/ports"
	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

// Shared wire values for the route use cases: every read is a GET and the
// shopper travels under one query/body key, so they cannot drift per route.
const (
	methodGet      = "GET"
	shopperIDParam = "shopperId"
)

// Actor is who a request acts for: the cookie state plus the Idempotency-Key
// the page sent for this submit (required on POST/PATCH at the edge).
type Actor struct {
	State          composition.State
	IdempotencyKey string
}

// subject is the stable owner part of a minted Idempotency-Key.
func (a Actor) subject() string {
	if a.State.ShopperID != "" {
		return a.State.ShopperID
	}
	if a.State.CartID != "" {
		return a.State.CartID
	}
	return "anonymous"
}

// Outcome is a use case result: the reply to relay, and the cookie state to
// store when the use case changed it.
type Outcome struct {
	Reply    composition.Reply
	State    composition.State
	SetState bool
}

// Gateway holds the composition use cases.
type Gateway struct {
	up      ports.Upstream
	budgets composition.Budgets
}

// NewGateway builds the use cases over an upstream port.
func NewGateway(up ports.Upstream, budgets composition.Budgets) *Gateway {
	return &Gateway{up: up, budgets: budgets}
}

// begin applies the route's time budget to the whole composition.
func (g *Gateway) begin(ctx context.Context, class composition.Class) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, g.budgets.For(class))
}

// call performs one allowlisted upstream call. A command (non-GET) made for a
// keyed request carries the upstream Idempotency-Key minted from the page's
// key; there is no retry: a failure is returned as is.
func (g *Gateway) call(ctx context.Context, a Actor, c composition.Call) (composition.Reply, error) {
	if !composition.Allowed(c) {
		return composition.Reply{}, composition.UpstreamError(composition.ErrRouteNotAllowed, c.Service)
	}
	if c.Method != methodGet && c.IdempotencyKey == "" && a.IdempotencyKey != "" {
		route := c.Method + " " + c.Path
		c.IdempotencyKey = composition.MintKey(route, a.subject(), a.IdempotencyKey)
	}
	reply, err := g.up.Do(ctx, c)
	if err != nil {
		return composition.Reply{}, classify(err, c.Service)
	}
	return reply, nil
}

func classify(err error, s composition.Service) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return composition.UpstreamError(composition.ErrUpstreamTimeout, s)
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	return composition.UpstreamError(composition.ErrUpstreamUnavailable, s)
}

// relay runs one call under the class budget and returns its reply untouched,
// including its Replayed flag.
func (g *Gateway) relay(ctx context.Context, class composition.Class, a Actor, c composition.Call) (Outcome, error) {
	ctx, cancel := g.begin(ctx, class)
	defer cancel()
	reply, err := g.call(ctx, a, c)
	return Outcome{Reply: reply}, err
}

// forward copies the named, non-empty query parameters from q into fwd.
func forward(fwd, q url.Values, names ...string) {
	for _, k := range names {
		if v := q.Get(k); v != "" {
			fwd.Set(k, v)
		}
	}
}
