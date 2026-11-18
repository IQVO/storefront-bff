// Consumer contract tests for the delivery-promise and pricing surfaces the
// BFF composes, against the byte-pinned testdata/delivery-promise.openapi.yaml
// and testdata/pricing.openapi.yaml. R1 routes, R2 fixtures, R3 the REAL
// client and gateway running the product page's DeliveryDates composition
// (quote joined with the pricing charge schedule), R4 the wire fields the
// decodes read.
package usecases

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

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
	checkExampleSatisfiesSchema(t, doc, "delivery-promise.quote-consolidated.example.json", "QuoteResponse")
	checkExampleSatisfiesSchema(t, doc, "delivery-promise.estimate-request.example.json", "EstimateRequest")
	checkExampleSatisfiesSchema(t, doc, "delivery-promise.estimate.example.json", "EstimateResponse")
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

// R3 (estimates): DeliveryEstimates posts the EstimateRequest the producer's
// schema describes (destinationPostalPrefix and the items, no tiers, so the
// producer estimates the four classic tiers and never CONSOLIDATED), expects
// 200 and relays the estimate unchanged, capacityBand included. The producer
// stores nothing and ignores a key, so none is minted.
func TestDeliveryEstimatesFlowsThroughTheRealClient(t *testing.T) {
	doc := loadPinnedSpec(t, "delivery-promise.openapi.yaml")
	request := requestBodySchema(t, doc, http.MethodPost, "/delivery-estimates")
	g, log := newUpstreamGateway(t, composition.DefaultBudgets(), func(c loggedCall) composition.Reply {
		return jsonFixtureReply(t, http.StatusOK, "delivery-promise.estimate.example.json")
	})
	out, err := g.DeliveryEstimates(t.Context(), []byte(`{"postalPrefix":"787","items":[{"catalogItemId":"sku-9001","quantity":1},{"catalogItemId":"sku-9002","quantity":3}]}`))
	if err != nil {
		t.Fatal(err)
	}
	calls := log.to(composition.DeliveryPromise)
	if len(calls) != 1 || calls[0].Method != http.MethodPost || calls[0].Path != "/delivery-estimates" {
		t.Fatalf("calls = %+v, want one POST /delivery-estimates", calls)
	}
	if calls[0].IdempotencyKey != "" {
		t.Error("a safe estimate carries no Idempotency-Key: the producer ignores it")
	}
	checkBodySatisfiesSchema(t, request, []byte(calls[0].Body), "the estimate request")
	if got, want := bytes.TrimSpace([]byte(calls[0].Body)), bytes.TrimSpace(readContractExample(t, "delivery-promise.estimate-request.example.json")); !jsonEqual(t, got, want) {
		t.Errorf("sent = %s, want the request fixture %s", got, want)
	}
	if bytes.Contains([]byte(calls[0].Body), []byte("tiers")) || bytes.Contains([]byte(calls[0].Body), []byte("CONSOLIDATED")) {
		t.Errorf("sent = %s: the BFF must not request tiers or CONSOLIDATED yet (ADR 0007)", calls[0].Body)
	}
	if out.Reply.Status != http.StatusOK || !bytes.Equal(out.Reply.Body, readContractExample(t, "delivery-promise.estimate.example.json")) {
		t.Errorf("reply = %d %s, want the producer's 200 body relayed byte for byte", out.Reply.Status, out.Reply.Body)
	}
	var got struct {
		Estimates []struct {
			Options []struct {
				Window       struct{ Start, End, TimeZone string }
				CapacityBand string `json:"capacityBand"`
			} `json:"options"`
		} `json:"estimates"`
	}
	if err := json.Unmarshal(out.Reply.Body, &got); err != nil || got.Estimates[0].Options[0].CapacityBand != "TIGHT" || got.Estimates[0].Options[0].Window.Start == "" {
		t.Errorf("relayed estimate lost capacityBand or the start/end window: %v %s", err, out.Reply.Body)
	}
}

func jsonEqual(t *testing.T, a, b []byte) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(x, y)
}

// R3 (estimates): the producer's 422 and a 5xx pass through the real client
// untouched.
func TestDeliveryEstimatesRelaysProducerProblems(t *testing.T) {
	problem := composition.Reply{Status: http.StatusUnprocessableEntity, ContentType: "application/problem+json", Body: []byte(`{"type":"/problems/validation-failed","title":"Validation failed","status":422}`)}
	g, _ := newUpstreamGateway(t, composition.DefaultBudgets(), func(loggedCall) composition.Reply { return problem })
	out, err := g.DeliveryEstimates(t.Context(), []byte(`{"postalPrefix":"787","items":[{"catalogItemId":"sku-9001","quantity":1}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.Reply.Status != problem.Status || out.Reply.ContentType != problem.ContentType || !bytes.Equal(out.Reply.Body, problem.Body) {
		t.Errorf("reply = %d %s %s, want the producer's problem untouched", out.Reply.Status, out.Reply.ContentType, out.Reply.Body)
	}
}

// R3 (capacity band and CONSOLIDATED): a quote whose offers carry capacityBand
// and a CONSOLIDATED offer with consolidationEligible composes tiers that
// relay both; the tier without them stays as before.
func TestDeliveryDatesRelaysCapacityBandAndConsolidatedTier(t *testing.T) {
	g, log := newUpstreamGateway(t, composition.DefaultBudgets(), func(c loggedCall) composition.Reply {
		if c.Service == "pricing" {
			return jsonFixtureReply(t, http.StatusOK, "pricing.schedule.example.json")
		}
		return jsonFixtureReply(t, http.StatusOK, "delivery-promise.quote-consolidated.example.json")
	})
	out, err := g.DeliveryDates(t.Context(), keyed, "sku-9001", []byte(`{"postalPrefix":"787"}`))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Tiers []map[string]json.RawMessage `json:"tiers"`
	}
	if err := json.Unmarshal(out.Reply.Body, &got); err != nil || len(got.Tiers) != 3 {
		t.Fatalf("tiers: %v %s", err, out.Reply.Body)
	}
	want := []struct{ tier, band, eligible string }{
		{`"NEXT_DAY"`, `"SCARCE"`, ""}, {`"STANDARD"`, `"AMPLE"`, ""}, {`"CONSOLIDATED"`, "", "true"},
	}
	for i, w := range want {
		tier := got.Tiers[i]
		if string(tier["tier"]) != w.tier || string(tier["capacityBand"]) != w.band || string(tier["consolidationEligible"]) != w.eligible {
			t.Errorf("tier[%d] = %v, want %s band=%q eligible=%q", i, tier, w.tier, w.band, w.eligible)
		}
	}
	for _, c := range log.to(composition.DeliveryPromise) {
		if bytes.Contains([]byte(c.Body), []byte("CONSOLIDATED")) || bytes.Contains([]byte(c.Body), []byte("tiers")) || bytes.Contains([]byte(c.Body), []byte("consolidatedDeliveryDay")) {
			t.Errorf("quote request %s asks for the CONSOLIDATED tier; downstream must accept it first (ADR 0007)", c.Body)
		}
	}
}

// Golden: an existing offer (no capacityBand, no CONSOLIDATED) composes the
// same bytes it did before those fields were relayed. The golden file was
// produced by the code of origin/develop before this change.
func TestDeliveryDatesExistingOffersAreByteIdentical(t *testing.T) {
	g, _ := newUpstreamGateway(t, composition.DefaultBudgets(), quoteAndSchedule(t))
	out, err := g.DeliveryDates(t.Context(), keyed, "sku-9001", []byte(`{"postalPrefix":"787"}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := readContractExample(t, "delivery-dates.existing-offers.golden.json"); !bytes.Equal(out.Reply.Body, want) {
		t.Errorf("composed = %s\nwant     = %s", out.Reply.Body, want)
	}
}

// R4 (estimates): every field of the request the BFF sends is declared by the
// pinned EstimateRequest and EstimateItem schemas with a compatible type.
func TestDeliveryEstimateRequestFieldsWeSendAreSpecified(t *testing.T) {
	doc := loadPinnedSpec(t, "delivery-promise.openapi.yaml")
	checkDTOFieldsOfSchema(t, doc, "EstimateRequest", reflect.TypeOf(estimateUpstreamRequest{}), false)
	checkDTOFieldsOfSchema(t, doc, "EstimateItem", reflect.TypeOf(estimateItem{}), false)
}

// R4 (BFF contract): what the BFF documents at its own edge agrees with the
// producer fixtures it relays or composes: the estimate fixture satisfies the
// BFF's DeliveryEstimates schema, and the composed product-page answer for a
// quote with capacityBand and a CONSOLIDATED offer satisfies DeliveryDates.
func TestEdgeSchemasAcceptWhatTheBFFRelaysAndComposes(t *testing.T) {
	spec, err := openapi3.NewLoader().LoadFromFile("../../../apis/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	check := func(schema string, body []byte) {
		t.Helper()
		var v any
		if err := json.Unmarshal(body, &v); err != nil {
			t.Fatal(err)
		}
		ref := spec.Components.Schemas[schema]
		if ref == nil {
			t.Fatalf("apis/openapi.yaml has no %s schema", schema)
		}
		if err := ref.Value.VisitJSON(v); err != nil {
			t.Errorf("%s does not satisfy the BFF's %s: %v", body, schema, err)
		}
	}
	check("DeliveryEstimates", readContractExample(t, "delivery-promise.estimate.example.json"))
	check("DeliveryEstimatesRequest", []byte(`{"postalPrefix":"787","items":[{"catalogItemId":"sku-9001","quantity":1}]}`))
	composed, err := composeDates("787", readContractExample(t, "delivery-promise.quote-consolidated.example.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	check("DeliveryDates", composed)
	check("DeliveryDates", readContractExample(t, "delivery-dates.existing-offers.golden.json"))
}
