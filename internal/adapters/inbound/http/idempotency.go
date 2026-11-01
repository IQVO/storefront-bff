package http

import (
	"net/http"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

// The idempotency headers of the fleet REST guideline (section 2.7).
const (
	idempotencyKeyHeader = "Idempotency-Key"
	replayedHeader       = "Idempotency-Replayed"
)

var (
	pKeyRequired = problemKind{http.StatusBadRequest, "idempotency-key-required", "Idempotency-Key header is required"}
	pKeyInvalid  = problemKind{http.StatusBadRequest, "idempotency-key-invalid", "Idempotency-Key must be 1-128 visible ASCII characters"}
)

// requireKey guards a POST or PATCH route: the page must send its own
// Idempotency-Key (a fresh one per user submit, reused on retry). The BFF
// stores nothing: it mints the upstream key from it (composition.MintKey), so
// a replay or a 409 idempotency-key-reused comes from the provider and is
// relayed unchanged. PUT and DELETE routes are not wrapped.
func requireKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vals := r.Header.Values(idempotencyKeyHeader)
		switch {
		case len(vals) == 0 || vals[0] == "":
			writeProblem(w, pKeyRequired, "")
		case len(vals) > 1 || !composition.ValidClientKey(vals[0]):
			writeProblem(w, pKeyInvalid, "")
		default:
			next(w, r)
		}
	}
}
