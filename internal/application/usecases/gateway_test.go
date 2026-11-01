package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

func TestCallRefusesRouteOffAllowlist(t *testing.T) {
	g := newGateway(newFake())
	_, err := g.call(context.Background(), Actor{}, composition.Call{Service: composition.CustomerOrdering, Method: "GET", Path: "/orders/o1/ship-to"})
	if !errors.Is(err, composition.ErrRouteNotAllowed) {
		t.Fatalf("err = %v", err)
	}
}

func TestCallClassifiesTransportErrors(t *testing.T) {
	cases := []struct {
		in   error
		want error
	}{
		{context.DeadlineExceeded, composition.ErrUpstreamTimeout},
		{errors.New("connection refused"), composition.ErrUpstreamUnavailable},
		{context.Canceled, context.Canceled},
	}
	for _, c := range cases {
		f := newFake().fail("GET /products/p1", c.in)
		_, err := newGateway(f).GetProduct(context.Background(), "p1")
		if !errors.Is(err, c.want) {
			t.Errorf("%v: err = %v, want %v", c.in, err, c.want)
		}
	}
}

func TestCommandsCarryDeterministicKeys(t *testing.T) {
	f := newFake().on("POST /carts", 201, `{"cartId":"c1"}`)
	g := newGateway(f)
	a := Actor{State: composition.State{ShopperID: "s1"}}
	call := composition.Call{Service: composition.ShoppingCart, Method: "POST", Path: "/carts", Body: []byte(`{}`)}
	_, _ = g.call(context.Background(), a, call)
	_, _ = g.call(context.Background(), a, call)
	if f.calls[0].IdempotencyKey == "" || f.calls[0].IdempotencyKey != f.calls[1].IdempotencyKey {
		t.Fatalf("keys = %q / %q", f.calls[0].IdempotencyKey, f.calls[1].IdempotencyKey)
	}
	want := composition.MintKey("POST /carts", "s1", "", []byte(`{}`), fixedNow)
	if f.calls[0].IdempotencyKey != want {
		t.Fatalf("key %q, want %q", f.calls[0].IdempotencyKey, want)
	}
}

func TestActorSubject(t *testing.T) {
	cases := []struct {
		a    Actor
		want string
	}{
		{Actor{State: composition.State{ShopperID: "s", CartID: "c"}}, "s"},
		{Actor{State: composition.State{CartID: "c"}}, "c"},
		{Actor{}, "anonymous"},
	}
	for _, c := range cases {
		if got := c.a.subject(); got != c.want {
			t.Errorf("subject(%+v) = %q, want %q", c.a, got, c.want)
		}
	}
}
