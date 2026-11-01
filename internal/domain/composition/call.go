// Package composition holds the pure composition and edge rules of the
// storefront BFF: which upstream calls exist, which routes the storefront may
// reach, the per-route time budgets, the Idempotency-Key minting and the
// shopper/cart state carried in the cookie. It performs no I/O.
package composition

import "net/url"

// Service names one upstream bounded context (or the network context the
// storefront reaches through its Open Host Service routes).
type Service string

// The upstream services the BFF composes. Each has a mandatory base URL in the
// environment (see cmd/api).
const (
	ProductDiscovery Service = "product-discovery"
	DeliveryPromise  Service = "delivery-promise"
	ShoppingCart     Service = "shopping-cart"
	Checkout         Service = "checkout"
	CustomerOrdering Service = "customer-ordering"
	Pricing          Service = "pricing"
)

// Services lists every upstream service, in a stable order.
func Services() []Service {
	return []Service{ProductDiscovery, DeliveryPromise, ShoppingCart, Checkout, CustomerOrdering, Pricing}
}

// Call is one outbound request to an upstream service. Body is forwarded as
// is; nothing in the BFF reads or logs it beyond what a composition needs.
type Call struct {
	Service        Service
	Method         string
	Path           string
	Query          url.Values
	Body           []byte
	IdempotencyKey string
}

// Reply is an upstream answer, kept raw so problem+json bodies pass through.
type Reply struct {
	Status      int
	ContentType string
	Body        []byte
}

// OK reports whether the upstream answered with a 2xx status.
func (r Reply) OK() bool { return r.Status >= 200 && r.Status < 300 }
