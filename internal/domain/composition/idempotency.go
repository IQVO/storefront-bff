package composition

import (
	"crypto/sha256"
	"encoding/hex"
)

// MaxClientKeyLen is the longest Idempotency-Key the storefront accepts from
// the page (the fleet bound, REST guideline 2.7).
const MaxClientKeyLen = 128

// ValidClientKey reports whether a page-sent Idempotency-Key is acceptable:
// 1-128 visible ASCII characters (no spaces or control bytes).
func ValidClientKey(k string) bool {
	if k == "" || len(k) > MaxClientKeyLen {
		return false
	}
	for i := 0; i < len(k); i++ {
		if k[i] <= ' ' || k[i] > '~' {
			return false
		}
	}
	return true
}

// MintKey derives the Idempotency-Key forwarded to an upstream command from
// the page's own Idempotency-Key.
//
// The key is deterministic for (route, subject, clientKey): the page owns the
// identity of a user submit, so a retry with the same client key reaches the
// upstream with the same key and gets the stored answer instead of a second
// effect; the same client key with a different body reaches the upstream with
// the same key and a different fingerprint, and the upstream answers 409
// idempotency-key-reused, which the BFF relays unchanged. There is no time
// window and no body hash here: the upstream fingerprints the body.
func MintKey(route, subject, clientKey string) string {
	h := sha256.New()
	for _, f := range []string{route, subject, clientKey} {
		h.Write([]byte(f))
		h.Write([]byte{0})
	}
	return "sf-" + hex.EncodeToString(h.Sum(nil))[:40]
}
