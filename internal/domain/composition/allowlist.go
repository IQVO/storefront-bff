package composition

import "strings"

// allowed is the closed list of upstream routes the storefront may call: the
// network's Open Host Service routes the model lists for "shopper /
// storefront" plus the customer-ordering shopper routes (NG1-NG3) and the
// storefront contexts' own REST routes. "{}" is one path segment; a literal
// tail after it (":confirm") is a custom-verb suffix. Anything else, notably
// ship-to, POST /promises, promise confirm/cancel, inventory feeds and policy
// routes, is refused before a request is built.
var allowed = map[Service][]string{
	ProductDiscovery: {"GET /products", "GET /products/{}", "GET /categories"},
	DeliveryPromise:  {"POST /delivery-offers:quote"},
	Pricing:          {"GET /delivery-charge-schedules/current"},
	ShoppingCart: {
		"POST /carts", "GET /carts", "GET /carts/{}", "POST /carts/{}/lines",
		"POST /carts/{}/lines/{}:set-quantity", "POST /carts/{}/lines/{}:remove",
		"POST /carts/{}:set-destination", "POST /carts/{}:quote-delivery", "POST /carts/{}:select-delivery",
	},
	Checkout: {"POST /checkout-sessions", "GET /checkout-sessions/{}", "POST /checkout-sessions/{}:confirm"},
	CustomerOrdering: {
		"GET /orders", "GET /orders/{}", "GET /orders/{}/tracking", "POST /orders/{}:cancel",
		"GET /shoppers", "POST /shoppers", "GET /shoppers/{}", "POST /shoppers/{}/addresses",
	},
}

// AllowedRoutes returns the allowlist as "METHOD /path" templates, per service.
func AllowedRoutes() map[Service][]string {
	out := make(map[Service][]string, len(allowed))
	for s, routes := range allowed {
		out[s] = append([]string(nil), routes...)
	}
	return out
}

// Allowed reports whether the call targets a route on the allowlist.
func Allowed(c Call) bool {
	for _, tpl := range allowed[c.Service] {
		method, path, ok := strings.Cut(tpl, " ")
		if ok && method == c.Method && pathMatches(path, c.Path) {
			return true
		}
	}
	return false
}

func pathMatches(template, path string) bool {
	tSegs := strings.Split(template, "/")
	pSegs := strings.Split(path, "/")
	if len(tSegs) != len(pSegs) {
		return false
	}
	for i, t := range tSegs {
		if !segmentMatches(t, pSegs[i]) {
			return false
		}
	}
	return true
}

func segmentMatches(template, seg string) bool {
	if !strings.HasPrefix(template, "{}") {
		return template == seg
	}
	id := strings.TrimSuffix(seg, strings.TrimPrefix(template, "{}"))
	return ValidID(id)
}
