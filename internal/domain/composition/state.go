package composition

import (
	"net/url"
	"strings"
)

// CookieName is the cookie that carries the shopper's browsing state. It is
// state, NOT authentication: nothing in the storefront trusts it for access.
const CookieName = "sf_state"

const maxIDLen = 128

// State is what the cookie remembers: the open cart, the shopper and the last
// checkout session (to join order totals). Every field is an opaque id.
type State struct {
	CartID     string
	ShopperID  string
	CheckoutID string
}

// ValidID reports whether s can be an upstream resource id: 1..128 characters
// of [A-Za-z0-9._-] starting with an alphanumeric. It keeps ids from altering
// the upstream path (no "/", no ":" verb, no dot segments).
func ValidID(s string) bool {
	if s == "" || len(s) > maxIDLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		alnum := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
		if !alnum && (i == 0 || (c != '.' && c != '_' && c != '-')) {
			return false
		}
	}
	return true
}

// Encode serialises the state as a query-string; empty fields are omitted.
func (s State) Encode() string {
	v := url.Values{}
	set := func(k, id string) {
		if id != "" {
			v.Set(k, id)
		}
	}
	set("c", s.CartID)
	set("s", s.ShopperID)
	set("k", s.CheckoutID)
	return v.Encode()
}

// DecodeState parses a cookie value. Malformed input and ids that fail
// ValidID are dropped, never an error: a bad cookie is an empty state.
func DecodeState(raw string) State {
	v, err := url.ParseQuery(raw)
	if err != nil {
		return State{}
	}
	pick := func(k string) string {
		id := strings.TrimSpace(v.Get(k))
		if ValidID(id) {
			return id
		}
		return ""
	}
	return State{CartID: pick("c"), ShopperID: pick("s"), CheckoutID: pick("k")}
}
