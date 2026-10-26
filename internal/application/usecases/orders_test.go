package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"testing"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

const orderBody = `{"orderId":"o1","shopperId":"s1","status":"PROMISED"}`

func orderFake() *fakeUp {
	return newFake().on("GET /orders/o1", 200, orderBody).
		on("GET /orders/o1/tracking", 200, `{"orderId":"o1","groups":[]}`).
		on("GET /checkout-sessions/k1", 200, `{"sessionId":"k1","orderId":"o1","currency":"USD","totalMinor":1999,"groups":[{"groupNo":1}]}`)
}

func decode(t *testing.T, b []byte) map[string]json.RawMessage {
	t.Helper()
	m := map[string]json.RawMessage{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%s: %v", b, err)
	}
	return m
}

func TestGetOrderJoinsTrackingAndTotals(t *testing.T) {
	a := Actor{State: composition.State{ShopperID: "s1", CheckoutID: "k1"}}
	out, err := newGateway(orderFake()).GetOrder(context.Background(), a, "o1")
	if err != nil || out.Reply.Status != 200 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	m := decode(t, out.Reply.Body)
	if string(m["orderId"]) != `"o1"` || m["tracking"] == nil {
		t.Fatalf("joined = %s", out.Reply.Body)
	}
	totals := decode(t, m["totals"])
	if string(totals["totalMinor"]) != "1999" || string(totals["currency"]) != `"USD"` || string(totals["checkoutId"]) != `"k1"` {
		t.Fatalf("totals = %s", m["totals"])
	}
}

func TestGetOrderOmitsTotalsOfAnotherOrder(t *testing.T) {
	f := orderFake().on("GET /checkout-sessions/k1", 200, `{"sessionId":"k1","orderId":"other"}`)
	a := Actor{State: composition.State{CheckoutID: "k1"}}
	out, _ := newGateway(f).GetOrder(context.Background(), a, "o1")
	if decode(t, out.Reply.Body)["totals"] != nil {
		t.Fatalf("totals of another order leaked: %s", out.Reply.Body)
	}
	if out, _ = newGateway(orderFake()).GetOrder(context.Background(), Actor{}, "o1"); decode(t, out.Reply.Body)["totals"] != nil {
		t.Fatal("no checkout in the state means no totals")
	}
}

func TestGetOrderDegradesWhenTrackingAndCheckoutFail(t *testing.T) {
	f := orderFake().fail("GET /orders/o1/tracking", errors.New("down")).fail("GET /checkout-sessions/k1", errors.New("down"))
	out, err := newGateway(f).GetOrder(context.Background(), Actor{State: composition.State{CheckoutID: "k1"}}, "o1")
	if err != nil {
		t.Fatal(err)
	}
	if m := decode(t, out.Reply.Body); m["tracking"] != nil || m["totals"] != nil || m["orderId"] == nil {
		t.Fatalf("degraded = %s", out.Reply.Body)
	}
}

func TestGetOrderFailures(t *testing.T) {
	g := newGateway(newFake())
	if out, err := g.GetOrder(context.Background(), Actor{}, "o404"); err != nil || out.Reply.Status != 404 {
		t.Errorf("missing order: out=%+v err=%v", out, err)
	}
	if _, err := g.GetOrder(context.Background(), Actor{}, "o/1"); !errors.Is(err, composition.ErrInvalid) {
		t.Errorf("bad id: %v", err)
	}
	f := newFake().on("GET /orders/o1", 200, `[1]`)
	if _, err := newGateway(f).GetOrder(context.Background(), Actor{}, "o1"); !errors.Is(err, composition.ErrUpstreamUnavailable) {
		t.Errorf("garbled order: %v", err)
	}
	f = newFake().fail("GET /orders/o1", context.DeadlineExceeded)
	if _, err := newGateway(f).GetOrder(context.Background(), Actor{}, "o1"); !errors.Is(err, composition.ErrUpstreamTimeout) {
		t.Errorf("timeout: %v", err)
	}
	f = orderFake().on("GET /orders/o1/tracking", 200, `not json`)
	if out, _ := newGateway(f).GetOrder(context.Background(), Actor{}, "o1"); decode(t, out.Reply.Body)["tracking"] != nil {
		t.Error("garbled tracking must be dropped")
	}
}

func TestListOrders(t *testing.T) {
	f := newFake().on("GET /orders", 200, `{"items":[]}`)
	a := Actor{State: composition.State{ShopperID: "s1"}}
	if _, err := newGateway(f).ListOrders(context.Background(), a, url.Values{"limit": {"5"}, "after": {"cur"}, "z": {"1"}}); err != nil {
		t.Fatal(err)
	}
	if got := f.last().Query.Encode(); got != "after=cur&limit=5&shopperId=s1" {
		t.Fatalf("query = %s", got)
	}
	if _, err := newGateway(f).ListOrders(context.Background(), Actor{}, nil); !errors.Is(err, composition.ErrNoSession) {
		t.Fatalf("no shopper: %v", err)
	}
}

func TestCancelOrder(t *testing.T) {
	f := newFake().on("POST /orders/o1:cancel", 200, orderBody)
	g := newGateway(f)
	if out, err := g.CancelOrder(context.Background(), Actor{}, "o1", []byte(`{"reason":"changed my mind"}`)); err != nil || out.Reply.Status != 200 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if f.last().IdempotencyKey == "" {
		t.Fatal("cancel is a command and needs a key")
	}
	for _, body := range []string{`{"reason":""}`, `{}`, `{"reason":"x","y":1}`, ``} {
		if _, err := g.CancelOrder(context.Background(), Actor{}, "o1", []byte(body)); !errors.Is(err, composition.ErrInvalid) {
			t.Errorf("body %q: %v", body, err)
		}
	}
	tooLong := make([]byte, 201)
	for i := range tooLong {
		tooLong[i] = 'a'
	}
	if _, err := g.CancelOrder(context.Background(), Actor{}, "o1", []byte(`{"reason":"`+string(tooLong)+`"}`)); !errors.Is(err, composition.ErrInvalid) {
		t.Errorf("201 chars: %v", err)
	}
}
