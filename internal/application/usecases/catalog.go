package usecases

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

// searchParams are the product-search filters the BFF forwards; anything else
// in the query string is dropped.
var searchParams = []string{"q", "categoryId", "inStock", "priceMin", "priceMax", "sort", "limit", "cursor"}

// SearchProducts relays GET /products to product-discovery.
func (g *Gateway) SearchProducts(ctx context.Context, q url.Values) (Outcome, error) {
	fwd := url.Values{}
	forward(fwd, q, searchParams...)
	return g.relay(ctx, composition.Query, Actor{}, composition.Call{
		Service: composition.ProductDiscovery, Method: methodGet, Path: "/products", Query: fwd,
	})
}

// ListCategories relays GET /categories to product-discovery: the category
// tree (id, parent, name, path) with purchasable counts, relayed unchanged.
// The upstream takes no parameters, so nothing from the query string is
// forwarded.
func (g *Gateway) ListCategories(ctx context.Context) (Outcome, error) {
	return g.relay(ctx, composition.Query, Actor{}, composition.Call{
		Service: composition.ProductDiscovery, Method: methodGet, Path: "/categories",
	})
}

// GetProduct relays GET /products/{id} to product-discovery.
func (g *Gateway) GetProduct(ctx context.Context, id string) (Outcome, error) {
	if !composition.ValidID(id) {
		return Outcome{}, composition.Invalid("product id is malformed")
	}
	return g.relay(ctx, composition.Query, Actor{}, composition.Call{
		Service: composition.ProductDiscovery, Method: methodGet, Path: "/products/" + id,
	})
}

type deliveryDatesRequest struct {
	PostalPrefix string `json:"postalPrefix"`
}

// validPostalPrefix accepts 1-10 letters, digits, spaces or dashes: a prefix,
// never a full address.
func validPostalPrefix(s string) bool {
	if s == "" || len(s) > 10 {
		return false
	}
	for _, r := range s {
		ok := r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r == ' ' || r == '-'
		if !ok {
			return false
		}
	}
	return strings.TrimSpace(s) != ""
}

// DeliveryDates quotes one unit of the product for a postal prefix and joins
// the per-tier delivery charge. The quote is required; a missing charge
// schedule only leaves the charge out.
// The Idempotency-Replayed flag of the quote (the one write hop) rides on the
// composed answer.
func (g *Gateway) DeliveryDates(ctx context.Context, a Actor, productID string, body []byte) (Outcome, error) {
	var req deliveryDatesRequest
	if err := decodeStrict(body, &req); err != nil || !validPostalPrefix(req.PostalPrefix) || !composition.ValidID(productID) {
		return Outcome{}, composition.Invalid("postalPrefix is required (a postal prefix, not an address) and the product id must be valid")
	}
	ctx, cancel := g.begin(ctx, composition.Query)
	defer cancel()
	quote, err := json.Marshal(map[string]any{
		"destinationPostalPrefix": req.PostalPrefix,
		"lines":                   []map[string]any{{"catalogItemId": productID, "quantity": 1}},
	})
	if err != nil {
		return Outcome{}, err
	}
	reply, err := g.call(ctx, a, composition.Call{
		Service: composition.DeliveryPromise, Method: "POST", Path: "/delivery-quotes", Body: quote,
	})
	if err != nil || !reply.OK() {
		return Outcome{Reply: reply}, err
	}
	charges := g.chargesByTier(ctx)
	out, err := composeDates(req.PostalPrefix, reply.Body, charges)
	if err != nil {
		return Outcome{}, composition.UpstreamError(composition.ErrUpstreamUnavailable, composition.DeliveryPromise)
	}
	composed := jsonReply(200, out)
	composed.Replayed = reply.Replayed
	return Outcome{Reply: composed}, nil
}
