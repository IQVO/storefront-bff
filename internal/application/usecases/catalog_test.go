package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

func TestSearchProductsForwardsOnlyKnownFilters(t *testing.T) {
	f := newFake().on("GET /products", 200, `{"items":[]}`)
	q := url.Values{"q": {"lamp"}, "limit": {"5"}, "evil": {"1"}, "sort": {""}}
	out, err := newGateway(f).SearchProducts(context.Background(), q)
	if err != nil || out.Reply.Status != 200 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	got := f.last()
	if got.Service != composition.ProductDiscovery || got.Query.Encode() != "limit=5&q=lamp" {
		t.Fatalf("call = %+v", got)
	}
	if got.IdempotencyKey != "" {
		t.Fatal("a query carries no Idempotency-Key")
	}
}

const categoriesBody = `{"version":3,"categories":[{"categoryId":"home","name":"Home","path":"Home","purchasableCount":4},{"categoryId":"lamps","parentId":"home","name":"Lamps","path":"Home/Lamps","purchasableCount":2}]}`

func TestListCategoriesRelaysTheTreeUnchanged(t *testing.T) {
	f := newFake().on("GET /categories", 200, categoriesBody)
	out, err := newGateway(f).ListCategories(context.Background())
	if err != nil || out.Reply.Status != 200 || string(out.Reply.Body) != categoriesBody {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	got := f.last()
	if got.Service != composition.ProductDiscovery || got.Method != "GET" || got.Path != "/categories" || len(got.Query) != 0 {
		t.Fatalf("call = %+v", got)
	}
	if got.IdempotencyKey != "" {
		t.Fatal("a query carries no Idempotency-Key")
	}
}

func TestListCategoriesPassesUpstreamFailuresThrough(t *testing.T) {
	f := newFake().on("GET /categories", 503, `{"type":"/problems/upstream-unavailable"}`)
	out, err := newGateway(f).ListCategories(context.Background())
	if err != nil || out.Reply.Status != 503 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}

func TestGetProductRejectsBadID(t *testing.T) {
	_, err := newGateway(newFake()).GetProduct(context.Background(), "a/b")
	if !errors.Is(err, composition.ErrInvalid) {
		t.Fatalf("err = %v", err)
	}
	f := newFake().on("GET /products/p1", 200, `{}`)
	if out, err := newGateway(f).GetProduct(context.Background(), "p1"); err != nil || out.Reply.Status != 200 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}

const quoteBody = `{"offers":[
 {"state":"QUOTED","tier":"NEXT_DAY","window":{"earliest":"2026-10-27T00:00:00Z","latest":"2026-10-27T23:59:00Z"},"expiresAt":"2026-10-26T12:10:00Z"},
 {"state":"EXPIRED","tier":"SAME_DAY","window":{"earliest":"a","latest":"b"},"expiresAt":"c"}],
 "unavailable":[{"tier":"SAME_DAY","constraint":"PAST_ORDER_DEADLINE"}]}`

const scheduleBody = `{"tiers":[{"tier":"NEXT_DAY","charge":{"amountMinor":599,"currency":"USD"},"freeOver":{"amountMinor":5000,"currency":"USD"}}]}`

func TestDeliveryDatesComposesQuoteAndCharges(t *testing.T) {
	f := newFake().on("POST /delivery-offers:quote", 200, quoteBody).on("GET /delivery-charge-schedules/current", 200, scheduleBody)
	out, err := newGateway(f).DeliveryDates(context.Background(), "p1", []byte(`{"postalPrefix":"787"}`))
	if err != nil || out.Reply.Status != 200 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	var got struct {
		PostalPrefix string `json:"postalPrefix"`
		Tiers        []struct {
			Tier, DeliverByEarliest string
			Charge                  map[string]any
		}
		Unavailable []map[string]any
	}
	if err := json.Unmarshal(out.Reply.Body, &got); err != nil {
		t.Fatal(err)
	}
	if got.PostalPrefix != "787" || len(got.Tiers) != 1 || got.Tiers[0].Tier != "NEXT_DAY" || got.Tiers[0].Charge["amountMinor"] != float64(599) || len(got.Unavailable) != 1 {
		t.Fatalf("composed = %s", out.Reply.Body)
	}
	q := f.calls[0]
	if q.Service != composition.DeliveryPromise || !strings.Contains(string(q.Body), `"destinationPostalPrefix":"787"`) ||
		!strings.Contains(string(q.Body), `"catalogItemId":"p1","quantity":1`) || q.IdempotencyKey == "" {
		t.Fatalf("quote call = %+v body=%s", q, q.Body)
	}
}

// The destination zone rides on each tier as quoted; a window quoted before
// delivery-promise carried zones has none, and the tier then omits the field.
func TestDeliveryDatesRelaysWindowTimeZone(t *testing.T) {
	quote := `{"offers":[
 {"state":"QUOTED","tier":"NEXT_DAY","window":{"earliest":"2026-10-31T14:00:00Z","latest":"2026-11-01T02:00:00Z","timeZone":"America/Chicago"},"expiresAt":"2026-10-30T12:10:00Z"},
 {"state":"QUOTED","tier":"STANDARD","window":{"earliest":"2026-11-03T00:00:00Z","latest":"2026-11-04T23:59:00Z"},"expiresAt":"2026-10-30T12:10:00Z"},
 {"state":"EXPIRED","tier":"SAME_DAY","window":{"earliest":"a","latest":"b","timeZone":"America/Denver"},"expiresAt":"c"}],
 "unavailable":[]}`
	f := newFake().on("POST /delivery-offers:quote", 200, quote).on("GET /delivery-charge-schedules/current", 200, scheduleBody)
	out, err := newGateway(f).DeliveryDates(context.Background(), "p1", []byte(`{"postalPrefix":"606"}`))
	if err != nil || out.Reply.Status != 200 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	var got struct {
		Tiers []map[string]json.RawMessage `json:"tiers"`
	}
	if err := json.Unmarshal(out.Reply.Body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Tiers) != 2 {
		t.Fatalf("tiers = %s", out.Reply.Body)
	}
	next, std := got.Tiers[0], got.Tiers[1]
	if string(next["timeZone"]) != `"America/Chicago"` || string(next["deliverByEarliest"]) != `"2026-10-31T14:00:00Z"` ||
		string(next["deliverByLatest"]) != `"2026-11-01T02:00:00Z"` {
		t.Fatalf("zoned tier = %s", out.Reply.Body)
	}
	if _, ok := std["timeZone"]; ok {
		t.Fatalf("a window without a zone must omit timeZone, got %s", out.Reply.Body)
	}
}

func TestDeliveryDatesWithoutChargeScheduleStillRenders(t *testing.T) {
	f := newFake().on("POST /delivery-offers:quote", 200, quoteBody).fail("GET /delivery-charge-schedules/current", errors.New("down"))
	out, err := newGateway(f).DeliveryDates(context.Background(), "p1", []byte(`{"postalPrefix":"787"}`))
	if err != nil || strings.Contains(string(out.Reply.Body), `"charge"`) {
		t.Fatalf("out=%s err=%v", out.Reply.Body, err)
	}
}

func TestDeliveryDatesRelaysUpstreamProblem(t *testing.T) {
	f := newFake().on("POST /delivery-offers:quote", 422, `{"type":"/problems/validation-failed"}`)
	out, err := newGateway(f).DeliveryDates(context.Background(), "p1", []byte(`{"postalPrefix":"787"}`))
	if err != nil || out.Reply.Status != 422 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}

func TestDeliveryDatesValidation(t *testing.T) {
	g := newGateway(newFake())
	for _, body := range []string{``, `{}`, `{"postalPrefix":""}`, `{"postalPrefix":"   "}`, `{"postalPrefix":"12345678901"}`,
		`{"postalPrefix":"78/7"}`, `{"postalPrefix":"787","street":"x"}`, `{"postalPrefix":"787"} x`} {
		if _, err := g.DeliveryDates(context.Background(), "p1", []byte(body)); !errors.Is(err, composition.ErrInvalid) {
			t.Errorf("body %q: err = %v", body, err)
		}
	}
	if _, err := g.DeliveryDates(context.Background(), "a/b", []byte(`{"postalPrefix":"787"}`)); !errors.Is(err, composition.ErrInvalid) {
		t.Errorf("bad product id: err = %v", err)
	}
}

func TestDeliveryDatesGarbledQuoteIsUnavailable(t *testing.T) {
	f := newFake().on("POST /delivery-offers:quote", 200, `not json`)
	_, err := newGateway(f).DeliveryDates(context.Background(), "p1", []byte(`{"postalPrefix":"787"}`))
	if !errors.Is(err, composition.ErrUpstreamUnavailable) {
		t.Fatalf("err = %v", err)
	}
}
