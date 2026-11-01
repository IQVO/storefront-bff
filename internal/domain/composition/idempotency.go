package composition

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// KeyWindow is the retry window of a minted Idempotency-Key: the same user
// action repeated inside one window carries the same key.
const KeyWindow = 10 * time.Minute

const maxActionLen = 64

// MintKey derives the Idempotency-Key forwarded to an upstream command.
//
// The key is deterministic for (route, subject, action, window), so a browser
// retry of the same action reaches the upstream with the same key and gets the
// stored answer instead of a second effect. With no client action id the
// request body stands in for the action, so two different payloads on one
// route never share a key. A client action id (a valid id, set by the page per
// click) replaces the body and is not windowed: the page owns its identity.
func MintKey(route, subject, clientAction string, body []byte, now time.Time) string {
	h := sha256.New()
	field := func(b []byte) {
		h.Write(b)
		h.Write([]byte{0})
	}
	field([]byte(route))
	field([]byte(subject))
	if clientAction != "" && ValidID(clientAction) && len(clientAction) <= maxActionLen {
		field([]byte("action"))
		field([]byte(clientAction))
	} else {
		sum := sha256.Sum256(body)
		field(sum[:])
		field([]byte(now.UTC().Truncate(KeyWindow).Format(time.RFC3339)))
	}
	return "sf-" + hex.EncodeToString(h.Sum(nil))[:40]
}
