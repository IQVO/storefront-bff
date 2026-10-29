package usecases

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

func TestCreateCartStoresCartIDInState(t *testing.T) {
	f := newFake().on("POST /carts", 201, `{"cartId":"c-9","state":"OPEN"}`)
	a := Actor{State: composition.State{ShopperID: "s1"}}
	out, err := newGateway(f).CreateCart(context.Background(), a, nil)
	if err != nil || !out.SetState || out.State.CartID != "c-9" || out.State.ShopperID != "s1" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if string(f.last().Body) != `{"shopperId":"s1"}` {
		t.Fatalf("empty body must adopt the cookie shopper, got %s", f.last().Body)
	}
}

func TestCreateCartKeepsClientBodyAndIgnoresFailures(t *testing.T) {
	f := newFake().on("POST /carts", 422, `{"type":"/problems/validation-failed"}`)
	out, err := newGateway(f).CreateCart(context.Background(), Actor{}, []byte(`{"shopperId":"x"}`))
	if err != nil || out.SetState || out.Reply.Status != 422 || string(f.last().Body) != `{"shopperId":"x"}` {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	f = newFake().on("POST /carts", 201, `{"cartId":"a/b"}`)
	out, _ = newGateway(f).CreateCart(context.Background(), Actor{}, nil)
	if out.SetState || string(f.last().Body) != `{}` {
		t.Fatalf("a malformed cart id must not enter the state: %+v", out)
	}
	f = newFake().on("POST /carts", 201, `nope`)
	if out, _ = newGateway(f).CreateCart(context.Background(), Actor{}, nil); out.SetState {
		t.Fatal("an unreadable reply must not set state")
	}
}

func TestCartCommandsMapToUpstreamVerbs(t *testing.T) {
	cases := []struct {
		name string
		run  func(*Gateway) (Outcome, error)
		want string
	}{
		{"add", func(g *Gateway) (Outcome, error) { return g.AddLine(context.Background(), Actor{}, "c1", []byte(`{}`)) }, "POST /carts/c1/lines"},
		{"dest", func(g *Gateway) (Outcome, error) { return g.SetDestination(context.Background(), Actor{}, "c1", nil) }, "POST /carts/c1:set-destination"},
		{"quote", func(g *Gateway) (Outcome, error) { return g.QuoteDelivery(context.Background(), Actor{}, "c1") }, "POST /carts/c1:quote-delivery"},
		{"select", func(g *Gateway) (Outcome, error) { return g.SelectDelivery(context.Background(), Actor{}, "c1", nil) }, "POST /carts/c1:select-delivery"},
		{"qty", func(g *Gateway) (Outcome, error) {
			return g.SetLineQuantity(context.Background(), Actor{}, "c1", "i1", nil)
		}, "POST /carts/c1/lines/i1:set-quantity"},
		{"remove", func(g *Gateway) (Outcome, error) { return g.RemoveLine(context.Background(), Actor{}, "c1", "i1") }, "POST /carts/c1/lines/i1:remove"},
	}
	for _, c := range cases {
		f := newFake().on(c.want, 200, `{}`)
		out, err := c.run(newGateway(f))
		if err != nil || out.Reply.Status != 200 {
			t.Errorf("%s: out=%+v err=%v", c.name, out, err)
			continue
		}
		if got := f.last(); got.Method+" "+got.Path != c.want || got.IdempotencyKey == "" || got.Service != composition.ShoppingCart {
			t.Errorf("%s: call = %+v", c.name, got)
		}
	}
}

func TestCartCommandsRejectBadIDs(t *testing.T) {
	g := newGateway(newFake())
	if _, err := g.AddLine(context.Background(), Actor{}, "../x", nil); !errors.Is(err, composition.ErrInvalid) {
		t.Errorf("cart id: %v", err)
	}
	if _, err := g.RemoveLine(context.Background(), Actor{}, "c1", "i:1"); !errors.Is(err, composition.ErrInvalid) {
		t.Errorf("item id: %v", err)
	}
	if _, err := g.GetCart(context.Background(), "a b"); !errors.Is(err, composition.ErrInvalid) {
		t.Errorf("get: %v", err)
	}
}

func TestListCartsDefaultsToCookieShopper(t *testing.T) {
	f := newFake().on("GET /carts", 200, `{"items":[]}`)
	a := Actor{State: composition.State{ShopperID: "s1"}}
	if _, err := newGateway(f).ListCarts(context.Background(), a, url.Values{"state": {"OPEN"}, "x": {"y"}}); err != nil {
		t.Fatal(err)
	}
	if got := f.last().Query.Encode(); got != "shopperId=s1&state=OPEN" {
		t.Fatalf("query = %s", got)
	}
	if _, err := newGateway(f).ListCarts(context.Background(), a, url.Values{"shopperId": {"s2"}}); err != nil || !strings.Contains(f.last().Query.Encode(), "s2") {
		t.Fatalf("explicit shopper must win: %v", f.last().Query)
	}
	if _, err := newGateway(f).ListCarts(context.Background(), Actor{}, nil); !errors.Is(err, composition.ErrInvalid) {
		t.Fatalf("no shopper: %v", err)
	}
}

func TestGetCartRelays(t *testing.T) {
	f := newFake().on("GET /carts/c1", 200, `{"cartId":"c1"}`)
	if out, err := newGateway(f).GetCart(context.Background(), "c1"); err != nil || out.Reply.Status != 200 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}
