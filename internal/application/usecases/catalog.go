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

// Limits of one batch estimate: the producer's own (delivery-promise ADR 0019).
const (
	maxEstimateItems    = 50
	maxEstimateItemID   = 64
	maxEstimateQuantity = 999
)

// estimateItem is one listing tile: the item and the quantity it is shown at.
type estimateItem struct {
	CatalogItemID string `json:"catalogItemId"`
	Quantity      int    `json:"quantity"`
}

// deliveryEstimatesRequest is the edge request: the page's postalPrefix (as on
// delivery-date-quotes) and the items of one listing view. There is no tiers
// field: the producer then estimates the four classic tiers and never
// CONSOLIDATED, which it refuses here with 422 (ADR 0007).
type deliveryEstimatesRequest struct {
	PostalPrefix string         `json:"postalPrefix"`
	Items        []estimateItem `json:"items"`
}

// estimateUpstreamRequest is the producer's EstimateRequest as the BFF sends it.
type estimateUpstreamRequest struct {
	DestinationPostalPrefix string         `json:"destinationPostalPrefix"`
	Items                   []estimateItem `json:"items"`
}

// validEstimateItems accepts 1..50 items, each catalogItemId a valid id of at
// most 64 characters that appears once, each quantity 1..999.
func validEstimateItems(items []estimateItem) bool {
	if len(items) == 0 || len(items) > maxEstimateItems {
		return false
	}
	seen := make(map[string]struct{}, len(items))
	for _, it := range items {
		if !composition.ValidID(it.CatalogItemID) || len(it.CatalogItemID) > maxEstimateItemID ||
			it.Quantity < 1 || it.Quantity > maxEstimateQuantity {
			return false
		}
		if _, dup := seen[it.CatalogItemID]; dup {
			return false
		}
		seen[it.CatalogItemID] = struct{}{}
	}
	return true
}

// DeliveryEstimates relays the batch delivery estimate of a listing page to
// delivery-promise POST /delivery-estimates (G60). It is a query: the producer
// stores nothing, ignores any Idempotency-Key and reserves no capacity, so no
// key is minted and the answer is relayed unchanged (estimates[].options[]
// with window{start,end,timeZone}, basis, confidence, orderByAt,
// capacityBand, carrierArea, and estimates[].constraints[]). An estimate is
// advisory: it is not an offer and cannot back a promise.
func (g *Gateway) DeliveryEstimates(ctx context.Context, body []byte) (Outcome, error) {
	var req deliveryEstimatesRequest
	if err := decodeStrict(body, &req); err != nil || !validPostalPrefix(req.PostalPrefix) || !validEstimateItems(req.Items) {
		return Outcome{}, composition.Invalid("postalPrefix is required (a postal prefix, not an address) and items must list 1 to 50 distinct catalogItemIds with a quantity of 1 to 999")
	}
	upstream, err := json.Marshal(estimateUpstreamRequest{DestinationPostalPrefix: req.PostalPrefix, Items: req.Items})
	if err != nil {
		return Outcome{}, err
	}
	return g.relay(ctx, composition.Query, Actor{}, composition.Call{
		Service: composition.DeliveryPromise, Method: "POST", Path: "/delivery-estimates", Body: upstream,
	})
}
