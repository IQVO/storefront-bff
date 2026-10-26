package composition

import "testing"

func TestAllowedAcceptsEveryListedRoute(t *testing.T) {
	cases := []Call{
		{Service: ProductDiscovery, Method: "GET", Path: "/products"},
		{Service: ProductDiscovery, Method: "GET", Path: "/products/item-1"},
		{Service: DeliveryPromise, Method: "POST", Path: "/delivery-offers:quote"},
		{Service: Pricing, Method: "GET", Path: "/delivery-charge-schedules/current"},
		{Service: ShoppingCart, Method: "POST", Path: "/carts/c1/lines/i1:set-quantity"},
		{Service: ShoppingCart, Method: "POST", Path: "/carts/c1/lines/i1:remove"},
		{Service: ShoppingCart, Method: "POST", Path: "/carts/c1:select-delivery"},
		{Service: Checkout, Method: "POST", Path: "/checkout-sessions/s1:confirm"},
		{Service: CustomerOrdering, Method: "GET", Path: "/orders/o1/tracking"},
		{Service: CustomerOrdering, Method: "POST", Path: "/shoppers/sh1/addresses"},
	}
	for _, c := range cases {
		if !Allowed(c) {
			t.Errorf("%s %s %s should be allowed", c.Service, c.Method, c.Path)
		}
	}
}

func TestAllowedRefusesInOrgAndRestrictedRoutes(t *testing.T) {
	cases := []Call{
		{Service: CustomerOrdering, Method: "GET", Path: "/orders/o1/ship-to"},
		{Service: DeliveryPromise, Method: "POST", Path: "/promises"},
		{Service: DeliveryPromise, Method: "POST", Path: "/promises/p1:confirm"},
		{Service: DeliveryPromise, Method: "POST", Path: "/promise-policies"},
		{Service: ProductDiscovery, Method: "POST", Path: "/products"},
		{Service: ShoppingCart, Method: "GET", Path: "/carts/c1/lines/i1:remove"},
		{Service: ShoppingCart, Method: "POST", Path: "/carts/c1/lines/:remove"},
		{Service: ShoppingCart, Method: "POST", Path: "/carts/../orders/lines"},
		{Service: ShoppingCart, Method: "POST", Path: "/carts/c1:merge-into"},
		{Service: Pricing, Method: "POST", Path: "/price-quotes"},
		{Service: "inventory-storage", Method: "GET", Path: "/stock"},
	}
	for _, c := range cases {
		if Allowed(c) {
			t.Errorf("%s %s %s must be refused", c.Service, c.Method, c.Path)
		}
	}
}

func TestAllowedRoutesReturnsACopy(t *testing.T) {
	routes := AllowedRoutes()
	routes[Checkout][0] = "DELETE /everything"
	if !Allowed(Call{Service: Checkout, Method: "POST", Path: "/checkout-sessions"}) {
		t.Fatal("mutating the returned map changed the allowlist")
	}
}
