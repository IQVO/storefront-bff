// Package http is the inbound REST adapter: the storefront edge under
// /api/storefront. It maps routes to use cases, owns the cookie, the
// problem+json shape and the request middleware. It never logs bodies.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

// problemBase is the namespace of this service's RFC 9457 `type` URIs.
const problemBase = "/problems/"

type problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type problemKind struct {
	status int
	slug   string
	title  string
}

var (
	pMalformed   = problemKind{http.StatusBadRequest, "malformed-request", "Malformed request"}
	pNotFound    = problemKind{http.StatusNotFound, "resource-not-found", "Resource not found"}
	pTooLarge    = problemKind{http.StatusRequestEntityTooLarge, "payload-too-large", "Request body too large"}
	pValidation  = problemKind{http.StatusUnprocessableEntity, "validation-failed", "Validation failed"}
	pNoSession   = problemKind{http.StatusUnprocessableEntity, "session-required", "A shopper session is required"}
	pUnavailable = problemKind{http.StatusServiceUnavailable, "upstream-unavailable", "An upstream service is unavailable; retry with the same action"}
	pTimeout     = problemKind{http.StatusGatewayTimeout, "gateway-timeout", "The request exceeded its time budget"}
	pInternal    = problemKind{http.StatusInternalServerError, "internal-error", "Internal server error"}
	// statusClientClosed is the de-facto status for a client that went away.
	pCancelled = problemKind{499, "client-closed-request", "Client closed the request"}
)

func writeProblem(w http.ResponseWriter, k problemKind, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(k.status)
	_ = json.NewEncoder(w).Encode(problem{Type: problemBase + k.slug, Title: k.title, Status: k.status, Detail: detail})
}

// writeError maps a use case failure to its problem. Details never carry
// request or upstream data.
func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, composition.ErrInvalid):
		writeProblem(w, pValidation, strings.TrimPrefix(err.Error(), composition.ErrInvalid.Error()+": "))
	case errors.Is(err, composition.ErrNoSession):
		writeProblem(w, pNoSession, "")
	case errors.Is(err, composition.ErrUpstreamTimeout):
		writeProblem(w, pTimeout, "")
	case errors.Is(err, composition.ErrUpstreamUnavailable):
		writeProblem(w, pUnavailable, "")
	case errors.Is(err, context.Canceled):
		writeProblem(w, pCancelled, "")
	default:
		writeProblem(w, pInternal, "")
	}
}
