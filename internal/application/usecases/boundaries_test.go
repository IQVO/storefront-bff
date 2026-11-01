package usecases

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

func TestValidPostalPrefixCharacterAndLengthEdges(t *testing.T) {
	for _, ok := range []string{"0123456789", "a", "z", "A", "Z", "0", "9", "78 7", "K1A-0B1"} {
		if !validPostalPrefix(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", " ", "01234567890", "{", "[", "@", "`", ":", "/", "7;8"} {
		if validPostalPrefix(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestConfirmTokenLengthEdge(t *testing.T) {
	f := newFake().on("POST /checkout-sessions/k1:confirm", 200, `{}`)
	g := newGateway(f)
	ok := `{"paymentMethodToken":"` + strings.Repeat("t", maxTokenLen) + `"}`
	if _, err := g.ConfirmCheckout(context.Background(), shopperActor, "k1", []byte(ok)); err != nil {
		t.Fatalf("256 characters must pass: %v", err)
	}
	bad := `{"paymentMethodToken":"` + strings.Repeat("t", maxTokenLen+1) + `"}`
	if _, err := g.ConfirmCheckout(context.Background(), shopperActor, "k1", []byte(bad)); !errors.Is(err, composition.ErrInvalid) {
		t.Fatalf("257 characters must be refused: %v", err)
	}
}

func TestConfirmCardFieldIsNamedInTheRefusal(t *testing.T) {
	_, err := newGateway(newFake()).ConfirmCheckout(context.Background(), shopperActor, "k1", []byte(`{"paymentMethodToken":"t","cvc":"1"}`))
	if err == nil || !strings.Contains(err.Error(), "card data is refused") {
		t.Fatalf("err = %v", err)
	}
}

func TestReasonAndEmailLengthEdges(t *testing.T) {
	f := newFake().on("POST /orders/o1:cancel", 200, `{}`).on("GET /shoppers", 404, `{}`)
	g := newGateway(f)
	reason := `{"reason":"` + strings.Repeat("r", maxReasonLen) + `"}`
	if _, err := g.CancelOrder(context.Background(), Actor{}, "o1", []byte(reason)); err != nil {
		t.Fatalf("200 characters must pass: %v", err)
	}
	email := `{"email":"` + strings.Repeat("e", maxEmailLen) + `"}`
	if _, err := g.Login(context.Background(), Actor{}, []byte(email)); err != nil {
		t.Fatalf("254 characters must pass: %v", err)
	}
}

func TestOrderTotalsWithoutGroupsCarryAnEmptyList(t *testing.T) {
	f := orderFake().on("GET /checkout-sessions/k1", 200, `{"sessionId":"k1","orderId":"o1","currency":"USD","totalMinor":5}`)
	out, err := newGateway(f).GetOrder(context.Background(), Actor{State: composition.State{CheckoutID: "k1"}}, "o1")
	if err != nil || !strings.Contains(string(decode(t, out.Reply.Body)["totals"]), `"groups":[]`) {
		t.Fatalf("out=%s err=%v", out.Reply.Body, err)
	}
}
