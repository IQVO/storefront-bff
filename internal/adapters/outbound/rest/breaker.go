package rest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ErrCircuitOpen is returned (wrapped, naming the host) when a request is
// refused because that upstream's circuit breaker is open. The edge's
// classify() maps it to the 503 upstream-unavailable problem, so the shopper
// gets an immediate, retryable answer instead of waiting out the route budget.
var ErrCircuitOpen = errors.New("upstream circuit open")

// Breaker defaults: five consecutive failures trip a host's circuit; after the
// cooldown a single probe request tests the water before traffic resumes.
const (
	breakerThreshold = 5
	breakerCooldown  = 10 * time.Second
)

type breakerState uint8

const (
	breakerClosed breakerState = iota
	breakerOpen
	breakerHalfOpen // one probe request is in flight
)

// hostBreaker is the closed/open/half-open state machine for one upstream
// host. All methods are safe for concurrent use.
type hostBreaker struct {
	mu       sync.Mutex
	state    breakerState
	failures int
	openedAt time.Time
}

// allow reports whether a request may proceed. In the open state it also
// performs the half-open transition: the first request after the cooldown
// becomes the probe; everything else (including parallel requests while the
// probe is in flight) is refused until the probe's outcome is recorded.
func (b *hostBreaker) allow(now time.Time, cooldown time.Duration) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case breakerClosed:
		return true
	case breakerOpen:
		if now.Sub(b.openedAt) < cooldown {
			return false
		}
		b.state = breakerHalfOpen
		return true
	default: // breakerHalfOpen: a probe is already in flight
		return false
	}
}

// probeOutcome is what a request did to the breaker.
type probeOutcome uint8

const (
	outcomeSuccess probeOutcome = iota
	outcomeFailure
	// outcomeNeutral is an abandoned request (the caller went away): it says
	// nothing about the upstream's health and neither trips nor heals.
	outcomeNeutral
)

// record applies a request's outcome. success clears the failure count;
// failure counts towards the threshold and trips the circuit at it. In
// half-open the probe alone decides: one success closes, one failure reopens.
func (b *hostBreaker) record(o probeOutcome, now time.Time, threshold int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case breakerHalfOpen:
		if o == outcomeSuccess {
			b.state = breakerClosed
			b.failures = 0
			break
		}
		// A failed probe reopens for another full cooldown; an abandoned one
		// (neutral) reopens but keeps the old openedAt, so the cooldown that
		// already elapsed still counts and the next request probes again.
		b.state = breakerOpen
		if o == outcomeFailure {
			b.openedAt = now
		}
	case breakerClosed:
		switch o {
		case outcomeFailure:
			b.failures++
			if b.failures >= threshold {
				b.state = breakerOpen
				b.openedAt = now
			}
		case outcomeSuccess:
			b.failures = 0
		}
	case breakerOpen:
		// A late outcome of a request that raced the trip: ignore it.
	}
}

// breakerTransport wraps an inner RoundTripper with one breaker per upstream
// host so a failing service fails fast instead of consuming every route's
// time budget. Failures are transport errors (including the route budget
// expiring on the wire — a host that always blows the budget is down), 5xx
// replies, and failed half-open probes; 4xx and 2xx are successes. A request
// whose context was cancelled by its caller is neutral: the shopper left.
type breakerTransport struct {
	inner     http.RoundTripper
	threshold int
	cooldown  time.Duration
	now       func() time.Time

	mu      sync.Mutex
	perHost map[string]*hostBreaker
}

func (t *breakerTransport) breakerFor(host string) *hostBreaker {
	t.mu.Lock()
	defer t.mu.Unlock()
	b, ok := t.perHost[host]
	if !ok {
		b = &hostBreaker{state: breakerClosed}
		t.perHost[host] = b
	}
	return b
}

// RoundTrip refuses the call when the host's circuit is open, otherwise it
// forwards to the inner transport and records the outcome. It never reads,
// logs, or retains request or response bodies.
func (t *breakerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Host
	b := t.breakerFor(host)
	if !b.allow(t.now(), t.cooldown) {
		return nil, fmt.Errorf("upstream %s: %w", host, ErrCircuitOpen)
	}
	resp, err := t.inner.RoundTrip(req)
	b.record(t.outcomeOf(resp, err), t.now(), t.threshold)
	return resp, err
}

func (t *breakerTransport) outcomeOf(resp *http.Response, err error) probeOutcome {
	switch {
	case err != nil:
		if errors.Is(err, context.Canceled) {
			return outcomeNeutral
		}
		return outcomeFailure
	case resp.StatusCode >= http.StatusInternalServerError:
		return outcomeFailure
	default:
		return outcomeSuccess
	}
}
