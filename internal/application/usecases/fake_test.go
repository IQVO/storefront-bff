package usecases

import (
	"context"
	"sync"
	"time"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

// fakeUp is a scripted Upstream: replies are keyed by "METHOD path".
type fakeUp struct {
	mu      sync.Mutex
	replies map[string]composition.Reply
	errs    map[string]error
	calls   []composition.Call
}

func newFake() *fakeUp {
	return &fakeUp{replies: map[string]composition.Reply{}, errs: map[string]error{}}
}

func (f *fakeUp) on(key string, status int, body string) *fakeUp {
	f.replies[key] = composition.Reply{Status: status, ContentType: "application/json", Body: []byte(body)}
	return f
}

func (f *fakeUp) fail(key string, err error) *fakeUp {
	f.errs[key] = err
	return f
}

func (f *fakeUp) Do(_ context.Context, c composition.Call) (composition.Reply, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, c)
	key := c.Method + " " + c.Path
	if err, ok := f.errs[key]; ok {
		return composition.Reply{}, err
	}
	if r, ok := f.replies[key]; ok {
		return r, nil
	}
	return composition.Reply{Status: 404, Body: []byte(`{"type":"/problems/not-found"}`)}, nil
}

func (f *fakeUp) last() composition.Call { return f.calls[len(f.calls)-1] }

var fixedNow = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

func newGateway(f *fakeUp) *Gateway {
	return NewGateway(f, composition.DefaultBudgets(), func() time.Time { return fixedNow })
}
