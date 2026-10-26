package composition

import (
	"strings"
	"testing"
)

func TestValidID(t *testing.T) {
	good := []string{"a", "cart-1", "0f8fad5b-d9cb-469f-a165-7867e75dfd58", "A.b_c-d", strings.Repeat("a", 128)}
	bad := []string{"", "-a", ".a", "..", "a/b", "a:b", "a b", "a%2Fb", "é", strings.Repeat("a", 129)}
	for _, id := range good {
		if !ValidID(id) {
			t.Errorf("%q should be valid", id)
		}
	}
	for _, id := range bad {
		if ValidID(id) {
			t.Errorf("%q should be invalid", id)
		}
	}
}

func TestStateRoundTrip(t *testing.T) {
	in := State{CartID: "c-1", ShopperID: "s-2", CheckoutID: "k-3"}
	if got := DecodeState(in.Encode()); got != in {
		t.Fatalf("round trip = %+v, want %+v", got, in)
	}
	if got := (State{}).Encode(); got != "" {
		t.Fatalf("empty state encodes to %q", got)
	}
	if got := (State{CartID: "c-1"}).Encode(); got != "c=c-1" {
		t.Fatalf("cart-only state encodes to %q", got)
	}
}

func TestDecodeStateDropsGarbage(t *testing.T) {
	if got := DecodeState("c=a%2Fb&s=ok&k=../x"); got != (State{ShopperID: "ok"}) {
		t.Fatalf("got %+v", got)
	}
	if got := DecodeState("%zz"); got != (State{}) {
		t.Fatalf("malformed cookie decoded to %+v", got)
	}
	if got := DecodeState(" c = x"); got != (State{}) {
		t.Fatalf("got %+v", got)
	}
	if got := DecodeState("c=%20c1%20"); got.CartID != "c1" {
		t.Fatalf("trim: got %+v", got)
	}
}

func TestValidIDAcceptsEveryCharacterClassEdge(t *testing.T) {
	for _, id := range []string{"a", "z", "A", "Z", "0", "9", "azAZ09", "a.z_A-Z"} {
		if !ValidID(id) {
			t.Errorf("%q should be valid", id)
		}
	}
	for _, id := range []string{"{", "[", "@", "`", ":", "/", "a{", "a[", "a@", "a`", "a:", "a/"} {
		if ValidID(id) {
			t.Errorf("%q should be invalid", id)
		}
	}
}
