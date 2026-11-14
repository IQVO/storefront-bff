// Consumer contract tests for the product-discovery surface the BFF
// composes, against the byte-pinned testdata/product-discovery.openapi.yaml.
// The BFF relays this producer's answers untouched: R1 the routes and every
// query parameter the BFF forwards are declared, R2 the fixtures satisfy the
// pinned schemas, R3 the REAL client relays them byte-for-byte, and there is
// no R4 decode because the BFF decodes nothing — the relay IS the contract.
package usecases

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

// productDiscoveryCallSites is the closed surface the BFF calls on
// product-discovery.
var productDiscoveryCallSites = []struct{ method, path string }{
	{http.MethodGet, "/products"},
	{http.MethodGet, "/products/{catalogItemId}"},
	{http.MethodGet, "/categories"},
}

// R1: the pinned spec declares every (method, path) the BFF calls.
func TestProductDiscoveryPinnedSpecCoversEveryCallSite(t *testing.T) {
	doc := loadPinnedSpec(t, "product-discovery.openapi.yaml")
	for _, c := range productDiscoveryCallSites {
		checkRouteDeclared(t, doc, c.method, c.path)
	}
}

// R1 (the relay's parameters): every query parameter the BFF forwards on the
// search route is declared by the pinned spec, and every declared parameter
// is one the BFF knows — a renamed or added producer parameter is a contract
// change this test surfaces.
func TestProductDiscoverySearchParamsMatchThePinnedSpec(t *testing.T) {
	doc := loadPinnedSpec(t, "product-discovery.openapi.yaml")
	item := doc.Paths.Find("/products")
	if item == nil || item.Get == nil {
		t.Fatal("pinned spec has no GET /products")
	}
	declared := map[string]bool{}
	for _, p := range item.Get.Parameters {
		if p.Value != nil && p.Value.In == "query" {
			declared[p.Value.Name] = true
		}
	}
	for _, name := range searchParams {
		if !declared[name] {
			t.Errorf("the BFF forwards %q, the pinned spec does not declare it on GET /products", name)
		}
	}
	for name := range declared {
		if !forwardedSearchParam(name) {
			t.Errorf("the pinned spec declares %q on GET /products, the BFF never forwards it", name)
		}
	}
}

func forwardedSearchParam(name string) bool {
	for _, p := range searchParams {
		if p == name {
			return true
		}
	}
	return false
}

// R2: every fixture satisfies the pinned schema of the message it stands for.
func TestProductDiscoveryExamplesSatisfyPinnedSpec(t *testing.T) {
	doc := loadPinnedSpec(t, "product-discovery.openapi.yaml")
	checkExampleSatisfiesSchema(t, doc, "product-discovery.search.example.json", "ProductSearchResult")
	checkExampleSatisfiesSchema(t, doc, "product-discovery.product.example.json", "ProductView")
	checkExampleSatisfiesSchema(t, doc, "product-discovery.categories.example.json", "CategoryViewList")
}

// R3: SearchProducts relays the producer's search page through the REAL
// client, forwarding only the declared filters.
func TestSearchProductsFlowsThroughTheRealClient(t *testing.T) {
	g, log := newUpstreamGateway(t, composition.DefaultBudgets(), func(c loggedCall) composition.Reply {
		if c.Method == http.MethodGet && c.Path == "/products" {
			return jsonFixtureReply(t, http.StatusOK, "product-discovery.search.example.json")
		}
		return composition.Reply{}
	})
	out, err := g.SearchProducts(t.Context(), parseQuery("q=telescope&categoryId=cat-outdoors&inStock=true&priceMin=1000&priceMax=9000&sort=price&limit=20&cursor=c-1&injected=no"))
	if err != nil {
		t.Fatal(err)
	}
	calls := log.to(composition.ProductDiscovery)
	if len(calls) != 1 {
		t.Fatalf("calls = %+v, want one GET /products", calls)
	}
	q := parseQuery(calls[0].RawQuery)
	for _, k := range searchParams {
		if !q.Has(k) {
			t.Errorf("query %q lacks %s", calls[0].RawQuery, k)
		}
	}
	if q.Has("injected") {
		t.Error("a parameter outside the allowlist must not be forwarded")
	}
	if !bytes.Equal(out.Reply.Body, readContractExample(t, "product-discovery.search.example.json")) {
		t.Error("the search page must be relayed byte-for-byte")
	}
}

// R3: GetProduct and ListCategories relay the producer's shapes unchanged.
func TestCatalogReadsFlowThroughTheRealClient(t *testing.T) {
	g, log := newUpstreamGateway(t, composition.DefaultBudgets(), func(c loggedCall) composition.Reply {
		switch c.Path {
		case "/products/sku-9001":
			return jsonFixtureReply(t, http.StatusOK, "product-discovery.product.example.json")
		case "/categories":
			return jsonFixtureReply(t, http.StatusOK, "product-discovery.categories.example.json")
		}
		return composition.Reply{}
	})
	product, err := g.GetProduct(t.Context(), "sku-9001")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(product.Reply.Body, readContractExample(t, "product-discovery.product.example.json")) {
		t.Error("the product view must be relayed byte-for-byte")
	}
	categories, err := g.ListCategories(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(categories.Reply.Body, readContractExample(t, "product-discovery.categories.example.json")) {
		t.Error("the category tree must be relayed byte-for-byte")
	}
	if len(log.to(composition.ProductDiscovery)) != 2 {
		t.Errorf("calls = %d, want two reads", len(log.to(composition.ProductDiscovery)))
	}
}
