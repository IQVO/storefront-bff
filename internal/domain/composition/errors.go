package composition

import (
	"errors"
	"fmt"
)

// Failures of the edge itself. Upstream answers (including their problem
// bodies) are NOT errors: they travel in a Reply.
var (
	// ErrRouteNotAllowed means a call targets a route off the allowlist.
	ErrRouteNotAllowed = errors.New("upstream route is not on the storefront allowlist")
	// ErrUpstreamUnavailable means the upstream could not be reached or broke the protocol.
	ErrUpstreamUnavailable = errors.New("upstream unavailable")
	// ErrUpstreamTimeout means the route's time budget expired.
	ErrUpstreamTimeout = errors.New("upstream time budget expired")
	// ErrInvalid marks a request the edge refuses before calling any upstream.
	ErrInvalid = errors.New("invalid request")
	// ErrNoSession means the request needs a shopper or cart the cookie does not carry.
	ErrNoSession = errors.New("no shopper in the session state")
)

// Invalid wraps ErrInvalid with a detail safe to return to the client (it
// never contains request data).
func Invalid(detail string) error { return fmt.Errorf("%w: %s", ErrInvalid, detail) }

// UpstreamError names the service that failed while keeping the sentinel.
func UpstreamError(kind error, s Service) error { return fmt.Errorf("%w: %s", kind, s) }
