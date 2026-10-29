package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

func TestRegisterShopperPassesBodyThroughAndStoresID(t *testing.T) {
	f := newFake().on("POST /shoppers", 201, `{"shopperId":"s-7","email":"a@example.test"}`)
	body := []byte(`{"email":"a@example.test","savedAddresses":[{"line1":"1 Test Way"}]}`)
	out, err := newGateway(f).RegisterShopper(context.Background(), Actor{State: composition.State{CartID: "c1"}}, body)
	if err != nil || !out.SetState || out.State.ShopperID != "s-7" || out.State.CartID != "c1" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if string(f.last().Body) != string(body) {
		t.Fatalf("body altered: %s", f.last().Body)
	}
}

func TestRegisterShopperConflictSetsNoState(t *testing.T) {
	f := newFake().on("POST /shoppers", 409, `{"type":"/problems/shopper-exists"}`)
	out, err := newGateway(f).RegisterShopper(context.Background(), Actor{}, []byte(`{}`))
	if err != nil || out.SetState || out.Reply.Status != 409 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	f = newFake().on("POST /shoppers", 201, `{"shopperId":"a/b"}`)
	if out, _ = newGateway(f).RegisterShopper(context.Background(), Actor{}, nil); out.SetState {
		t.Fatal("a malformed id must not enter the state")
	}
	f = newFake().fail("POST /shoppers", errors.New("down"))
	if _, err = newGateway(f).RegisterShopper(context.Background(), Actor{}, nil); !errors.Is(err, composition.ErrUpstreamUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestLoginLooksUpByEmail(t *testing.T) {
	f := newFake().on("GET /shoppers", 200, `{"shopperId":"s-1"}`)
	out, err := newGateway(f).Login(context.Background(), Actor{}, []byte(`{"email":"a@example.test"}`))
	if err != nil || out.State.ShopperID != "s-1" || !out.SetState {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if f.last().Query.Get("email") != "a@example.test" || f.last().IdempotencyKey != "" {
		t.Fatalf("call = %+v", f.last())
	}
}

func TestLoginRefusals(t *testing.T) {
	g := newGateway(newFake())
	for _, body := range []string{``, `{}`, `{"email":""}`, `{"email":"a","x":1}`} {
		if _, err := g.Login(context.Background(), Actor{}, []byte(body)); !errors.Is(err, composition.ErrInvalid) {
			t.Errorf("body %q: %v", body, err)
		}
	}
	long := make([]byte, 255)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := g.Login(context.Background(), Actor{}, []byte(`{"email":"`+string(long)+`"}`)); !errors.Is(err, composition.ErrInvalid) {
		t.Errorf("255 chars: %v", err)
	}
	if out, err := g.Login(context.Background(), Actor{}, []byte(`{"email":"nobody@example.test"}`)); err != nil || out.Reply.Status != 404 || out.SetState {
		t.Errorf("unknown shopper: out=%+v err=%v", out, err)
	}
}

func TestMeAndAddAddressNeedAShopper(t *testing.T) {
	g := newGateway(newFake())
	if _, err := g.Me(context.Background(), Actor{}); !errors.Is(err, composition.ErrNoSession) {
		t.Errorf("me: %v", err)
	}
	if _, err := g.AddAddress(context.Background(), Actor{}, nil); !errors.Is(err, composition.ErrNoSession) {
		t.Errorf("address: %v", err)
	}
}

func TestMeAndAddAddressRelay(t *testing.T) {
	f := newFake().on("GET /shoppers/s1", 200, `{}`).on("POST /shoppers/s1/addresses", 201, `{"addressId":"a1"}`)
	g := newGateway(f)
	a := Actor{State: composition.State{ShopperID: "s1"}}
	if out, err := g.Me(context.Background(), a); err != nil || out.Reply.Status != 200 {
		t.Fatalf("me: out=%+v err=%v", out, err)
	}
	addr := []byte(`{"line1":"1 Test Way"}`)
	if out, err := g.AddAddress(context.Background(), a, addr); err != nil || out.Reply.Status != 201 {
		t.Fatalf("address: out=%+v err=%v", out, err)
	}
	if string(f.last().Body) != string(addr) || f.last().IdempotencyKey == "" {
		t.Fatalf("call = %+v", f.last())
	}
}
