package rest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

// fakeTransport stands in for the wire: it counts calls and answers with the
// configured error or status without any network.
type fakeTransport struct {
	mu     sync.Mutex
	calls  int
	status int // 0 with err set means fail; otherwise answer with this status
	err    error
}

func (f *fakeTransport) RoundTrip(*http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &http.Response{StatusCode: f.status, Body: http.NoBody}, nil
}

func (f *fakeTransport) succeed() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err, f.status = nil, http.StatusOK
}

func (f *fakeTransport) failWith(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeTransport) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// newTestBreaker wires a breakerTransport over fake with a manual clock, so
// cooldowns advance without sleeping.
func newTestBreaker(fake *fakeTransport, threshold int, cooldown time.Duration) (*breakerTransport, *time.Time) {
	now := time.Unix(0, 0)
	return &breakerTransport{
		inner: fake, threshold: threshold, cooldown: cooldown,
		now: func() time.Time { return now }, perHost: map[string]*hostBreaker{},
	}, &now
}

// trip drives one request through the breaker, closing any reply body, and
// returns just the error (the canonical bodyclose-safe shape).
func trip(t *testing.T, bt *breakerTransport, req *http.Request) error {
	t.Helper()
	resp, err := bt.RoundTrip(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return nil
}

func wantTrip(t *testing.T, bt *breakerTransport, host string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+host+"/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

// Threshold consecutive transport failures trip the circuit; the next request
// is refused at the breaker without reaching the wire.
func TestBreakerTripsAfterConsecutiveFailures(t *testing.T) {
	fake := &fakeTransport{err: errors.New("connection refused")}
	bt, _ := newTestBreaker(fake, 3, time.Minute)
	req := wantTrip(t, bt, "catalog:1")
	for i := 0; i < 3; i++ {
		if err := trip(t, bt, req); err == nil {
			t.Fatalf("call %d: expected the wire error", i)
		}
	}
	if fake.count() != 3 {
		t.Fatalf("wire calls = %d", fake.count())
	}
	err := trip(t, bt, req)
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("err = %v, want ErrCircuitOpen", err)
	}
	if fake.count() != 3 {
		t.Fatalf("fast-fail reached the wire: calls = %d", fake.count())
	}
}

// 5xx replies count as failures; 2xx/4xx do not.
func TestBreakerCounts5xxAsFailures(t *testing.T) {
	fake := &fakeTransport{status: http.StatusServiceUnavailable}
	bt, _ := newTestBreaker(fake, 2, time.Minute)
	req := wantTrip(t, bt, "pricing:1")
	for i := 0; i < 2; i++ {
		resp, err := bt.RoundTrip(req)
		if err != nil || resp.StatusCode != 503 {
			t.Fatalf("call %d: resp=%v err=%v", i, resp, err)
		}
		_ = resp.Body.Close()
	}
	if err := trip(t, bt, req); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("err = %v, want ErrCircuitOpen after two 503s", err)
	}
}

// A success resets the consecutive-failure count, so intermittent errors under
// the threshold never trip the circuit.
func TestBreakerSuccessResetsFailureCount(t *testing.T) {
	fake := &fakeTransport{err: errors.New("connection reset")}
	bt, _ := newTestBreaker(fake, 3, time.Minute)
	req := wantTrip(t, bt, "carts:1")
	for i := 0; i < 2; i++ {
		_ = trip(t, bt, req)
	}
	fake.succeed()
	if err := trip(t, bt, req); err != nil {
		t.Fatalf("success call: %v", err)
	}
	fake.failWith(errors.New("connection reset"))
	for i := 0; i < 2; i++ {
		if err := trip(t, bt, req); err == nil || errors.Is(err, ErrCircuitOpen) {
			t.Fatalf("call %d after reset: %v (circuit must still be closed)", i, err)
		}
	}
}

// After the cooldown the first request is a probe: success closes the circuit.
func TestBreakerHalfOpenProbeSuccessCloses(t *testing.T) {
	fake := &fakeTransport{err: errors.New("boom")}
	bt, now := newTestBreaker(fake, 1, time.Minute)
	req := wantTrip(t, bt, "checkout:1")
	_ = trip(t, bt, req) // trips with threshold 1
	*now = now.Add(2 * time.Minute)

	fake.succeed()
	if err := trip(t, bt, req); err != nil {
		t.Fatalf("probe: %v", err)
	}
	if err := trip(t, bt, req); err != nil {
		t.Fatalf("circuit must be closed after a healthy probe: %v", err)
	}
}

// A failed probe reopens for a full cooldown; a second probe may then retry.
func TestBreakerHalfOpenProbeFailureReopens(t *testing.T) {
	fake := &fakeTransport{err: errors.New("boom")}
	bt, now := newTestBreaker(fake, 1, time.Minute)
	req := wantTrip(t, bt, "checkout:1")
	_ = trip(t, bt, req)
	*now = now.Add(2 * time.Minute)

	if err := trip(t, bt, req); err == nil || errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("probe must reach the wire and fail there: %v", err)
	}
	if err := trip(t, bt, req); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("err = %v, want the reopened circuit to refuse", err)
	}
	*now = now.Add(time.Minute)
	fake.succeed()
	if err := trip(t, bt, req); err != nil {
		t.Fatalf("second probe: %v", err)
	}
}

// A request cancelled by its caller is neutral: it neither trips nor heals,
// and an abandoned half-open probe does not burn a fresh cooldown.
func TestBreakerCallerCancelIsNeutral(t *testing.T) {
	canceled := fmt.Errorf("net/http: request canceled: %w", context.Canceled)
	fake := &fakeTransport{err: canceled}
	bt, now := newTestBreaker(fake, 1, time.Minute)
	req := wantTrip(t, bt, "orders:1")
	for i := 0; i < 3; i++ {
		if err := trip(t, bt, req); !errors.Is(err, context.Canceled) {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if err := trip(t, bt, req); !errors.Is(err, context.Canceled) {
		t.Fatalf("call %d: %v", 3, err)
	}
	fake.succeed()
	if err := trip(t, bt, req); err != nil {
		t.Fatalf("cancellations must not trip the circuit: %v", err)
	}

	// Now trip it, wait out the cooldown, and abandon the probe.
	fake.failWith(errors.New("boom"))
	_ = trip(t, bt, req)
	*now = now.Add(2 * time.Minute)
	fake.failWith(canceled)
	_ = trip(t, bt, req) // abandoned probe
	fake.succeed()
	if err := trip(t, bt, req); err != nil {
		t.Fatalf("an abandoned probe must not restart the cooldown: %v", err)
	}
}

// Circuits are per host: one failing upstream never blocks a healthy sibling.
func TestBreakerIsPerHost(t *testing.T) {
	fake := &fakeTransport{err: errors.New("boom")}
	bt, _ := newTestBreaker(fake, 1, time.Minute)
	down := wantTrip(t, bt, "down:1")
	up := wantTrip(t, bt, "up:1")
	_ = trip(t, bt, down)
	if err := trip(t, bt, down); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("down host: %v", err)
	}
	fake.succeed()
	if err := trip(t, bt, up); err != nil {
		t.Fatalf("healthy host must be unaffected: %v", err)
	}
}

// Through the real Client (otelhttp + breaker over the wire): after the
// threshold consecutive 500s the next call fails fast with ErrCircuitOpen and
// never reaches the server.
func TestClientFailsFastWhenUpstreamBreakerOpens(t *testing.T) {
	var hits int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c, err := New(bases(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	call := composition.Call{Service: composition.ShoppingCart, Method: http.MethodGet, Path: "/carts"}
	for i := 0; i < breakerThreshold; i++ {
		if reply, err := c.Do(context.Background(), call); err != nil || reply.Status != 500 {
			t.Fatalf("call %d: reply=%+v err=%v", i, reply, err)
		}
	}
	_, err = c.Do(context.Background(), call)
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("err = %v, want ErrCircuitOpen", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != breakerThreshold {
		t.Fatalf("server hits = %d, want %d (the refused call must not reach the wire)", hits, breakerThreshold)
	}
}
