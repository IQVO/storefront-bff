package composition

import (
	"strings"
	"testing"
)

func TestMintKeyIsDeterministicForTheClientKey(t *testing.T) {
	a := MintKey("POST /carts", "shopper-1", "click-1")
	b := MintKey("POST /carts", "shopper-1", "click-1")
	if a != b {
		t.Fatalf("the same client key minted %q and %q", a, b)
	}
	if !strings.HasPrefix(a, "sf-") || len(a) != 43 {
		t.Fatalf("unexpected key shape %q", a)
	}
}

func TestMintKeyDiffersAcrossInputs(t *testing.T) {
	base := MintKey("POST /carts", "s1", "k1")
	variants := map[string]string{
		"route":   MintKey("POST /orders", "s1", "k1"),
		"subject": MintKey("POST /carts", "s2", "k1"),
		"key":     MintKey("POST /carts", "s1", "k2"),
	}
	for name, k := range variants {
		if k == base {
			t.Errorf("changing the %s did not change the key", name)
		}
	}
}

func TestMintKeyFieldsCannotBleed(t *testing.T) {
	if MintKey("ab", "c", "k") == MintKey("a", "bc", "k") {
		t.Fatal("route/subject boundary must be delimited")
	}
	if MintKey("r", "sk", "") == MintKey("r", "s", "k") {
		t.Fatal("subject/key boundary must be delimited")
	}
}

func TestValidClientKey(t *testing.T) {
	cases := map[string]bool{
		"":                       false,
		"k-1":                    true,
		"7d3f0c1e-uuid":          true,
		strings.Repeat("a", 128): true,
		strings.Repeat("a", 129): false,
		"has space":              false,
		"tab\tkey":               false,
		"caf\u00e9":              false,
	}
	for k, want := range cases {
		if got := ValidClientKey(k); got != want {
			t.Errorf("ValidClientKey(%q) = %v, want %v", k, got, want)
		}
	}
}
