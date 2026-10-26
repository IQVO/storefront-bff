package composition

import (
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 26, 10, 3, 0, 0, time.UTC)

func TestMintKeyIsDeterministicWithinWindow(t *testing.T) {
	a := MintKey("POST /carts", "shopper-1", "", []byte(`{"x":1}`), t0)
	b := MintKey("POST /carts", "shopper-1", "", []byte(`{"x":1}`), t0.Add(5*time.Minute))
	if a != b {
		t.Fatalf("same action in one window minted %q and %q", a, b)
	}
	if !strings.HasPrefix(a, "sf-") || len(a) != 43 {
		t.Fatalf("unexpected key shape %q", a)
	}
}

func TestMintKeyDiffersAcrossInputs(t *testing.T) {
	base := MintKey("POST /carts", "s1", "", []byte("a"), t0)
	variants := map[string]string{
		"route":   MintKey("POST /orders", "s1", "", []byte("a"), t0),
		"subject": MintKey("POST /carts", "s2", "", []byte("a"), t0),
		"body":    MintKey("POST /carts", "s1", "", []byte("b"), t0),
		"window":  MintKey("POST /carts", "s1", "", []byte("a"), t0.Add(KeyWindow)),
	}
	for name, k := range variants {
		if k == base {
			t.Errorf("changing the %s did not change the key", name)
		}
	}
}

func TestMintKeyClientActionReplacesBodyAndWindow(t *testing.T) {
	a := MintKey("r", "s", "click-1", []byte("a"), t0)
	b := MintKey("r", "s", "click-1", []byte("b"), t0.Add(3*KeyWindow))
	if a != b {
		t.Fatal("a client action id must pin the key regardless of body and time")
	}
	if a == MintKey("r", "s", "click-2", []byte("a"), t0) {
		t.Fatal("different client actions must differ")
	}
}

func TestMintKeyIgnoresInvalidClientAction(t *testing.T) {
	bad := MintKey("r", "s", "has/slash", []byte("a"), t0)
	if bad != MintKey("r", "s", "", []byte("a"), t0) {
		t.Fatal("an invalid action id must fall back to the body")
	}
	long := strings.Repeat("a", 65)
	if MintKey("r", "s", long, []byte("a"), t0) != MintKey("r", "s", "", []byte("a"), t0) {
		t.Fatal("an over-long action id must fall back to the body")
	}
	ok := strings.Repeat("a", 64)
	if MintKey("r", "s", ok, []byte("a"), t0) == MintKey("r", "s", "", []byte("a"), t0) {
		t.Fatal("a 64-character action id is accepted")
	}
}

func TestMintKeyFieldsCannotBleed(t *testing.T) {
	if MintKey("ab", "c", "", nil, t0) == MintKey("a", "bc", "", nil, t0) {
		t.Fatal("route/subject boundary must be delimited")
	}
}
