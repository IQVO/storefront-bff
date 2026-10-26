package rest

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

func bases(url string) map[composition.Service]string {
	m := map[composition.Service]string{}
	for _, s := range composition.Services() {
		m[s] = url
	}
	return m
}

func TestNewRequiresEveryBaseURL(t *testing.T) {
	for name, mutate := range map[string]func(map[composition.Service]string){
		"missing":  func(m map[composition.Service]string) { delete(m, composition.Pricing) },
		"relative": func(m map[composition.Service]string) { m[composition.Checkout] = "/x" },
		"scheme":   func(m map[composition.Service]string) { m[composition.Checkout] = "ftp://h" },
		"garbage":  func(m map[composition.Service]string) { m[composition.Checkout] = "http://[::1" },
	} {
		m := bases("http://localhost:1")
		mutate(m)
		if _, err := New(m); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestDoSendsComposedRequest(t *testing.T) {
	var got *http.Request
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(422)
		_, _ = w.Write([]byte(`{"type":"/problems/x"}`))
	}))
	defer srv.Close()
	c, err := New(bases(srv.URL + "/base/"))
	if err != nil {
		t.Fatal(err)
	}
	reply, err := c.Do(context.Background(), composition.Call{
		Service: composition.ShoppingCart, Method: "POST", Path: "/carts/c1:quote-delivery",
		Body: []byte(`{"a":1}`), IdempotencyKey: "k-1",
	})
	if err != nil || reply.Status != 422 || reply.ContentType != "application/problem+json" || string(reply.Body) != `{"type":"/problems/x"}` {
		t.Fatalf("reply=%+v err=%v", reply, err)
	}
	if got.URL.Path != "/base/carts/c1:quote-delivery" || got.Header.Get("Idempotency-Key") != "k-1" ||
		got.Header.Get("Content-Type") != "application/json" || gotBody != `{"a":1}` {
		t.Fatalf("request = %s %v body=%s", got.URL, got.Header, gotBody)
	}
}

func TestDoOmitsBodyHeadersOnQueries(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r }))
	defer srv.Close()
	c, _ := New(bases(srv.URL))
	_, err := c.Do(context.Background(), composition.Call{
		Service: composition.ProductDiscovery, Method: "GET", Path: "/products", Query: map[string][]string{"q": {"a b"}},
	})
	if err != nil || got.URL.RawQuery != "q=a+b" || got.Header.Get("Content-Type") != "" || got.Header.Get("Idempotency-Key") != "" {
		t.Fatalf("request = %s %v err=%v", got.URL, got.Header, err)
	}
}

func TestDoDoesNotFollowRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:1/elsewhere", http.StatusFound)
	}))
	defer srv.Close()
	c, _ := New(bases(srv.URL))
	reply, err := c.Do(context.Background(), composition.Call{Service: composition.Checkout, Method: "GET", Path: "/checkout-sessions/x"})
	if err != nil || reply.Status != http.StatusFound {
		t.Fatalf("reply=%+v err=%v", reply, err)
	}
}

func TestDoHonoursContextBudget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()
	c, _ := New(bases(srv.URL))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.Do(ctx, composition.Call{Service: composition.Checkout, Method: "GET", Path: "/checkout-sessions/x"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestDoRejectsOversizeReply(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxReplyBytes+1)))
	}))
	defer srv.Close()
	c, _ := New(bases(srv.URL))
	if _, err := c.Do(context.Background(), composition.Call{Service: composition.Checkout, Method: "GET", Path: "/x"}); err == nil {
		t.Fatal("expected a size-cap error")
	}
}

func TestDoUnknownServiceAndBadMethod(t *testing.T) {
	c, _ := New(bases("http://localhost:1"))
	if _, err := c.Do(context.Background(), composition.Call{Service: "nope", Method: "GET", Path: "/"}); !errors.Is(err, composition.ErrRouteNotAllowed) {
		t.Errorf("unknown service: %v", err)
	}
	if _, err := c.Do(context.Background(), composition.Call{Service: composition.Checkout, Method: "BAD METHOD", Path: "/"}); err == nil {
		t.Error("a malformed method must fail")
	}
}
