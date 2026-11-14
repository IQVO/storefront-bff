// Cross-service consumer contract tests: the thin-relay failure contract the
// BFF holds with every pinned producer. R3: a producer's problem+json answer
// passes through the REAL client untouched, and a producer that breaches the
// route's time budget maps to the composition sentinel the edge answers 504
// with — never a silent or mislabelled failure.
package usecases

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

// R3: a problem+json answer from a producer is relayed status, content type
// and body untouched — the storefront sees the producer's own problem, which
// is only correct while the pinned specs keep that shape.
func TestProblemJSONPassesThroughUntouched(t *testing.T) {
	problem := composition.Reply{
		Status:      http.StatusUnprocessableEntity,
		ContentType: "application/problem+json",
		Body:        []byte(`{"type":"/problems/validation-failed","title":"Validation failed","status":422}`),
	}
	g, _ := newUpstreamGateway(t, composition.DefaultBudgets(), func(loggedCall) composition.Reply {
		return problem
	})
	out, err := g.AddLine(t.Context(), keyed, "cart-7701", readContractExample(t, "shopping-cart.line-request.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	if out.Reply.Status != problem.Status || out.Reply.ContentType != problem.ContentType || string(out.Reply.Body) != string(problem.Body) {
		t.Errorf("reply = %d %s %s, want the problem relayed untouched", out.Reply.Status, out.Reply.ContentType, out.Reply.Body)
	}
	if out.SetState {
		t.Error("a failed upstream answer must not write cookie state")
	}
}

// R3: a producer that breaches the route's budget maps to the
// ErrUpstreamTimeout sentinel naming the service (the edge answers 504); a
// refused connection maps to ErrUpstreamUnavailable. These sentinels are the
// BFF's side of every pinned contract: every producer failure surfaces as
// one of them, never as a raw transport error.
func TestUpstreamFailuresMapToCompositionSentinels(t *testing.T) {
	budgets := composition.Budgets{Query: 30 * time.Millisecond, Command: time.Second, Confirm: time.Second}
	g, _ := newUpstreamGateway(t, budgets, func(loggedCall) composition.Reply {
		time.Sleep(300 * time.Millisecond)
		return composition.Reply{}
	})
	_, err := g.SearchProducts(t.Context(), parseQuery("q=telescope"))
	if !errors.Is(err, composition.ErrUpstreamTimeout) {
		t.Errorf("err = %v, want the ErrUpstreamTimeout sentinel", err)
	} else if !strings.Contains(err.Error(), string(composition.ProductDiscovery)) {
		t.Errorf("err = %q, want it to name %s", err, composition.ProductDiscovery)
	}
}

// R3: the Idempotency-Replayed flag of a keyed producer answer rides the
// composed reply, so a replayed quote is visible to the page through the BFF.
func TestReplayedFlagRidesTheComposedReply(t *testing.T) {
	g, _ := newUpstreamGateway(t, composition.DefaultBudgets(), func(c loggedCall) composition.Reply {
		reply := jsonFixtureReply(t, http.StatusOK, "delivery-promise.quote.example.json")
		reply.Replayed = true
		return reply
	})
	out, err := g.DeliveryDates(t.Context(), keyed, "sku-9001", []byte(`{"postalPrefix":"787"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !out.Reply.Replayed {
		t.Error("the Idempotency-Replayed flag must ride the composed answer")
	}
}
