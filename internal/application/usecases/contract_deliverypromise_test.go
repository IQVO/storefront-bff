// Consumer contract tests for the delivery-promise and pricing surfaces the
// BFF composes, against the byte-pinned testdata/delivery-promise.openapi.yaml
// and testdata/pricing.openapi.yaml. R1 routes, R2 fixtures, R3 the REAL
// client and gateway running the product page's DeliveryDates composition
// (quote joined with the pricing charge schedule), R4 the wire fields the
// decodes read.
package usecases

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

// deliveryPromiseCallSites is the closed surface the BFF calls on
// delivery-promise (the allowlist quotes the product page and estimates a
// later wave of the same composition).
var deliveryPromiseCallSites = []struct{ method, path string }{
	{http.MethodPost, "/delivery-quotes"},
	{http.MethodPost, "/delivery-estimates"},
}

// R1: the pinned spec declares every (method, path) the BFF calls.
func TestDeliveryPromisePinnedSpecCoversEveryCallSite(t *testing.T) {
	doc := loadPinnedSpec(t, "delivery-promise.openapi.yaml")
	for _, c := range deliveryPromiseCallSites {
		checkRouteDeclared(t, doc, c.method, c.path)
	}
}

// R2: every fixture satisfies the pinned schema of the message it stands for.
func TestDeliveryPromiseExamplesSatisfyPinnedSpec(t *testing.T) {
	doc := loadPinnedSpec(t, "delivery-promise.openapi.yaml")
	checkExampleSatisfiesSchema(t, doc, "delivery-promise.quote.example.json", "QuoteResponse")
}

// R2/R1 (pricing): the schedule fixture satisfies the pinned schema, and the
// one route the BFF calls is declared.
func TestPricingPinnedSpecCoversEveryCallSite(t *testing.T) {
	doc := loadPinnedSpec(t, "pricing.openapi.yaml")
	checkRouteDeclared(t, doc, http.MethodGet, "/delivery-charge-schedules/current")
	checkExampleSatisfiesSchema(t, doc, "pricing.schedule.example.json", "DeliveryChargeSchedule")
}

// quoteAndSchedule answers a delivery-promise quote and a pricing schedule.
func quoteAndSchedule(t *testing.T) func(loggedCall) composition.Reply {
	t.Helper()
	return func(c loggedCall) composition.Reply {
		switch c.Service {
		case "delivery-promise":
			return jsonFixtureReply(t, http.StatusOK, "delivery-promise.quote.example.json")
		case "pricing":
			return jsonFixtureReply(t, http.StatusOK, "pricing.schedule.example.json")
		}
		return composition.Reply{}
	}
}

// R3: DeliveryDates posts the QuoteRequest the producer's schema describes
// (one unit of the product for a postal prefix), carries the minted
// Idempotency-Key the producer's quote route requires, and composes one tier
// per quoted offer with the pricing charge of the same tier name.
func TestDeliveryDatesFlowsThroughTheRealClient(t *testing.T) {
	doc := loadPinnedSpec(t, "delivery-promise.openapi.yaml")
	quote := requestBodySchema(t, doc, http.MethodPost, "/delivery-quotes")
	g, log := newUpstreamGateway(t, composition.DefaultBudgets(), quoteAndSchedule(t))
	out, err := g.DeliveryDates(t.Context(), keyed, "sku-9001", []byte(`{"postalPrefix":"787"}`))
	if err != nil {
		t.Fatal(err)
	}
	quotes := log.to(composition.DeliveryPromise)
	if len(quotes) != 1 || quotes[0].Method != http.MethodPost || quotes[0].Path != "/delivery-quotes" {
		t.Fatalf("calls = %+v, want one POST /delivery-quotes", quotes)
	}
	if quotes[0].IdempotencyKey == "" {
		t.Error("the quote route requires an Idempotency-Key; a keyed page submit must mint one")
	}
	checkBodySatisfiesSchema(t, quote, []byte(quotes[0].Body), "the quote request")
	var sent struct {
		DestinationPostalPrefix string `json:"destinationPostalPrefix"`
		Lines                   []struct {
			CatalogItemID string `json:"catalogItemId"`
			Quantity      int    `json:"quantity"`
		} `json:"lines"`
	}
	if err := json.Unmarshal([]byte(quotes[0].Body), &sent); err != nil {
		t.Fatal(err)
	}
	if sent.DestinationPostalPrefix != "787" || len(sent.Lines) != 1 || sent.Lines[0].CatalogItemID != "sku-9001" || sent.Lines[0].Quantity != 1 {
		t.Errorf("sent = %+v, want one unit of the product to the prefix", sent)
	}
	checkComposedDates(t, out.Reply.Body)
	if len(log.to(composition.Pricing)) != 1 {
		t.Errorf("pricing calls = %d, want the schedule read once", len(log.to(composition.Pricing)))
	}
}

// checkComposedDates asserts the composed product-page answer: two quoted
// tiers with their pricing charges and the producer's unavailable entry.
func checkComposedDates(t *testing.T, body []byte) {
	t.Helper()
	var got struct {
		PostalPrefix string `json:"postalPrefix"`
		Tiers        []struct {
			Tier     string          `json:"tier"`
			Earliest string          `json:"deliverByEarliest"`
			TimeZone string          `json:"timeZone"`
			Charge   json.RawMessage `json:"charge"`
			FreeOver json.RawMessage `json:"freeOver"`
		} `json:"tiers"`
		Unavailable []json.RawMessage `json:"unavailable"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("composed body: %v (%s)", err, body)
	}
	if got.PostalPrefix != "787" || len(got.Tiers) != 2 || len(got.Unavailable) != 1 {
		t.Fatalf("composed = %s", body)
	}
	if got.Tiers[0].Tier != "NEXT_DAY" || string(got.Tiers[0].Charge) != `{"amountMinor":599,"currency":"USD"}` {
		t.Errorf("tier[0] = %+v, want NEXT_DAY with pricing's 599 USD charge", got.Tiers[0])
	}
	if got.Tiers[1].Tier != "STANDARD" || len(got.Tiers[1].FreeOver) == 0 {
		t.Errorf("tier[1] = %+v, want STANDARD with pricing's freeOver relayed", got.Tiers[1])
	}
	if got.Tiers[0].TimeZone != "America/Chicago" || got.Tiers[0].Earliest != "2026-11-16T14:00:00Z" {
		t.Errorf("tier[0] window = %+v, want the quote's window relayed", got.Tiers[0])
	}
}

// R3: a quote without a readable charge schedule still renders its tiers —
// the pricing failure degrades to absent charges, never a failed page.
func TestDeliveryDatesDegradesWithoutTheSchedule(t *testing.T) {
	g, log := newUpstreamGateway(t, composition.DefaultBudgets(), func(c loggedCall) composition.Reply {
		if c.Service == "delivery-promise" {
			return jsonFixtureReply(t, http.StatusOK, "delivery-promise.quote.example.json")
		}
		return composition.Reply{Status: http.StatusServiceUnavailable, ContentType: "application/problem+json", Body: []byte(`{"type":"/problems/upstream-unavailable"}`)}
	})
	out, err := g.DeliveryDates(t.Context(), Actor{}, "sku-9001", []byte(`{"postalPrefix":"787"}`))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Tiers []struct {
			Tier   string          `json:"tier"`
			Charge json.RawMessage `json:"charge"`
		} `json:"tiers"`
	}
	if err := json.Unmarshal(out.Reply.Body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Tiers) != 2 {
		t.Fatalf("tiers = %s, want the quote's tiers without charges", out.Reply.Body)
	}
	for _, tier := range got.Tiers {
		if len(tier.Charge) != 0 {
			t.Errorf("tier %s carries a charge without a schedule: %s", tier.Tier, tier.Charge)
		}
	}
	if len(log.to(composition.Pricing)) != 1 {
		t.Errorf("pricing calls = %d, want one attempted read", len(log.to(composition.Pricing)))
	}
}

// R4: every wire field the quote decode reads is declared by the pinned
// schema. quotedOffer is the real decode type of dates.go; dateWindowFixture
// mirrors its inline window verbatim.
func TestDeliveryPromiseFieldsWeReadAreSpecified(t *testing.T) {
	doc := loadPinnedSpec(t, "delivery-promise.openapi.yaml")
	checkDTOFieldsOfSchema(t, doc, "DeliveryOffer", reflect.TypeOf(quotedOffer{}), true)
	checkDTOFieldsOfSchema(t, doc, "DeliveryWindow", reflect.TypeOf(dateWindowFixture{}), true)
}

// dateWindowFixture mirrors the inline window decode of quotedOffer.
type dateWindowFixture struct {
	Earliest string `json:"earliest"`
	Latest   string `json:"latest"`
	TimeZone string `json:"timeZone,omitempty"`
}

// R4 (pricing): every wire field the schedule decode reads is declared by
// the pinned schema; freeOver is optional and tolerated absent.
func TestPricingFieldsWeReadAreSpecified(t *testing.T) {
	doc := loadPinnedSpec(t, "pricing.openapi.yaml")
	checkDTOFieldsOfSchema(t, doc, "TierCharge", reflect.TypeOf(tierCharge{}), true)
}
