package composition

import (
	"errors"
	"testing"
)

func TestLooksLikeCardNumber(t *testing.T) {
	yes := []string{"4111111111111111", "4111 1111 1111 1111", "4111-1111-1111-1111", "378282246310005", "6011111111111117"}
	no := []string{"", "tok_abc123", "4111111111111112", "123456789012", "41111111111111111111", "4111x111111111111", "pm_1234567890123456"}
	for _, s := range yes {
		if !LooksLikeCardNumber(s) {
			t.Errorf("%q should look like a card number", s)
		}
	}
	for _, s := range no {
		if LooksLikeCardNumber(s) {
			t.Errorf("%q must not look like a card number", s)
		}
	}
}

func TestIsCardFieldName(t *testing.T) {
	for _, k := range []string{"pan", "CVC", "cardNumber", "Expiry", "cvv"} {
		if !IsCardFieldName(k) {
			t.Errorf("%q should be a card field", k)
		}
	}
	for _, k := range []string{"paymentMethodToken", "cartId", "panel"} {
		if IsCardFieldName(k) {
			t.Errorf("%q must not be a card field", k)
		}
	}
}

func TestErrorWrapping(t *testing.T) {
	if err := Invalid("bad"); !errors.Is(err, ErrInvalid) || err.Error() != "invalid request: bad" {
		t.Fatalf("Invalid: %v", err)
	}
	err := UpstreamError(ErrUpstreamTimeout, Checkout)
	if !errors.Is(err, ErrUpstreamTimeout) || err.Error() != "upstream time budget expired: checkout" {
		t.Fatalf("UpstreamError: %v", err)
	}
}

func TestLooksLikeCardNumberLengthBoundaries(t *testing.T) {
	// Luhn-valid numbers of 12, 13, 19 and 20 digits.
	cases := map[string]bool{
		"100000000008":         false,
		"1000000000009":        true,
		"1000000000000000009":  true,
		"10000000000000000008": false,
	}
	for in, want := range cases {
		if got := LooksLikeCardNumber(in); got != want {
			t.Errorf("%s (%d digits) = %v, want %v", in, len(in), got, want)
		}
	}
}
