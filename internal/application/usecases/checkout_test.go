package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

var shopperActor = Actor{State: composition.State{ShopperID: "s1", CartID: "c1"}}

func TestOpenCheckoutComposesSessionRequest(t *testing.T) {
	f := newFake().on("POST /checkout-sessions", 201, `{"sessionId":"k-1","state":"OPEN"}`)
	out, err := newGateway(f).OpenCheckout(context.Background(), shopperActor, []byte(`{"shipToAddressId":"a1"}`))
	if err != nil || !out.SetState || out.State.CheckoutID != "k-1" || out.State.CartID != "c1" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if got := string(f.last().Body); got != `{"cartId":"c1","shipToAddressId":"a1","shopperId":"s1"}` {
		t.Fatalf("body = %s", got)
	}
}

func TestOpenCheckoutRefusals(t *testing.T) {
	g := newGateway(newFake())
	ctx := context.Background()
	if _, err := g.OpenCheckout(ctx, Actor{}, []byte(`{}`)); !errors.Is(err, composition.ErrNoSession) {
		t.Errorf("no shopper: %v", err)
	}
	for _, body := range []string{``, `{}`, `{"shipToAddressId":"a/1"}`, `{"shipToAddressId":"a1","street":"x"}`, `not json`} {
		if _, err := g.OpenCheckout(ctx, shopperActor, []byte(body)); !errors.Is(err, composition.ErrInvalid) {
			t.Errorf("body %q: %v", body, err)
		}
	}
	noCart := Actor{State: composition.State{ShopperID: "s1"}}
	if _, err := g.OpenCheckout(ctx, noCart, []byte(`{"shipToAddressId":"a1"}`)); !errors.Is(err, composition.ErrInvalid) {
		t.Errorf("no cart: %v", err)
	}
}

func TestOpenCheckoutRelaysConflict(t *testing.T) {
	f := newFake().on("POST /checkout-sessions", 409, `{"type":"/problems/cart-not-checkout-ready"}`)
	out, err := newGateway(f).OpenCheckout(context.Background(), shopperActor, []byte(`{"cartId":"c2","shipToAddressId":"a1"}`))
	if err != nil || out.Reply.Status != 409 || out.SetState {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	f = newFake().on("POST /checkout-sessions", 201, `garbage`)
	if out, _ = newGateway(f).OpenCheckout(context.Background(), shopperActor, []byte(`{"shipToAddressId":"a1"}`)); out.SetState {
		t.Fatal("unreadable reply must not set state")
	}
}

func TestConfirmCheckoutUsesConfirmBudgetAndKeepsBody(t *testing.T) {
	f := newFake().on("POST /checkout-sessions/k1:confirm", 202, `{"state":"PLACING"}`)
	body := []byte(`{"paymentMethodToken":"tok_abc"}`)
	out, err := newGateway(f).ConfirmCheckout(context.Background(), shopperActor, "k1", body)
	if err != nil || out.Reply.Status != 202 || string(f.last().Body) != string(body) || f.last().IdempotencyKey == "" {
		t.Fatalf("out=%+v err=%v call=%+v", out, err, f.last())
	}
}

func TestConfirmCheckoutRefusesCardData(t *testing.T) {
	g := newGateway(newFake())
	for _, body := range []string{
		`{"paymentMethodToken":"tok","cvc":"123"}`, `{"pan":"4111111111111111"}`, `{"paymentMethodToken":"4111 1111 1111 1111"}`,
		`{"paymentMethodToken":""}`, `{}`, `[]`, ``, `{"paymentMethodToken":"tok","other":1}`,
	} {
		if _, err := g.ConfirmCheckout(context.Background(), shopperActor, "k1", []byte(body)); !errors.Is(err, composition.ErrInvalid) {
			t.Errorf("body %q must be refused: %v", body, err)
		}
	}
	if _, err := g.ConfirmCheckout(context.Background(), shopperActor, "k/1", []byte(`{"paymentMethodToken":"t"}`)); !errors.Is(err, composition.ErrInvalid) {
		t.Errorf("bad id: %v", err)
	}
}

func TestGetCheckout(t *testing.T) {
	f := newFake().on("GET /checkout-sessions/k1", 200, `{}`)
	if out, err := newGateway(f).GetCheckout(context.Background(), "k1"); err != nil || out.Reply.Status != 200 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if _, err := newGateway(f).GetCheckout(context.Background(), "k 1"); !errors.Is(err, composition.ErrInvalid) {
		t.Fatalf("err=%v", err)
	}
}
