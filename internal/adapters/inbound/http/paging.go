package http

import (
	"net/http"

	"github.com/IQVO/storefront-bff/internal/adapters/inbound/http/page"
)

var (
	pInvalidLimit = problemKind{http.StatusBadRequest, "invalid-limit", "Invalid limit"}
	pUnknownParam = problemKind{http.StatusBadRequest, "unknown-query-parameter", "Unknown query parameter"}
)

// pageParams validates a paged collection's query string before it is
// relayed: an undeclared parameter (notably the retired after, offset and
// nextToken) answers 400 unknown-query-parameter and a bad limit 400
// invalid-limit. The cursor is the provider's opaque token, relayed as is; the
// provider answers 400 invalid-cursor for a bad one.
func pageParams(w http.ResponseWriter, r *http.Request, filters ...string) bool {
	if name := page.Unknown(r, filters...); name != "" {
		writeProblem(w, pUnknownParam, "unknown query parameter "+name)
		return false
	}
	if _, err := page.Limit(r); err != nil {
		writeProblem(w, pInvalidLimit, err.Error())
		return false
	}
	return true
}
