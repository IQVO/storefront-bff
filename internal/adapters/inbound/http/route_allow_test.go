package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestWrongMethodCarriesAllowHeader pins RFC 9110 section 15.5.6: a 405 MUST
// list the methods the resource does support. chi sets Allow only in its
// DEFAULT 405 handler; once a custom handler is registered (the RFC 9457 one)
// chi calls it as-is, so the handler derives the list from the router itself.
func TestWrongMethodCarriesAllowHeader(t *testing.T) {
	cases := []struct{ method, path, wantAllow string }{
		{http.MethodDelete, "/healthz", "GET"},
		{http.MethodPost, "/readyz", "GET"},
		{http.MethodPut, "/carts", "GET, POST"},
		{http.MethodDelete, "/carts", "GET, POST"},
		{http.MethodDelete, "/carts/c1", "GET"},
		{http.MethodGet, "/carts/c1/destination", "PUT"},
		{http.MethodPost, "/carts/c1/lines/i1", "PATCH, DELETE"},
		{http.MethodGet, "/carts/c1/lines/i1", "PATCH, DELETE"},
		{http.MethodPut, "/products", "GET"},
		{http.MethodGet, "/checkout", "POST"},
		{http.MethodDelete, "/checkout/k1", "GET"},
		{http.MethodPut, BasePath + "/carts", "GET, POST"},
		{http.MethodPost, BasePath + "/healthz", "GET"},
	}
	h := newRig().h
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := serve(h, tc.method, tc.path)
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want 405", rec.Code)
			}
			if got := rec.Header().Get("Allow"); got != tc.wantAllow {
				t.Errorf("Allow = %q, want %q", got, tc.wantAllow)
			}
		})
	}
}

// TestMethodNotAllowedBodyUnchanged pins that adding Allow left the problem
// body and content type as they were.
func TestMethodNotAllowedBodyUnchanged(t *testing.T) {
	rec := serve(newRig().h, http.MethodPut, "/carts")
	const want = `{"type":"/problems/method-not-allowed","title":"Method not allowed","status":405}` + "\n"
	if got := rec.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("Content-Type = %q", got)
	}
}

// TestUnknownPathCarriesNoAllow: a 404 is not a 405 and lists no methods.
func TestUnknownPathCarriesNoAllow(t *testing.T) {
	rec := serve(newRig().h, http.MethodPut, "/nope")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != "" {
		t.Errorf("Allow = %q, want empty", got)
	}
}

// TestAllowedMethodsListsEveryRegisteredMethod covers a path with several
// methods, so the list is derived from the router and not hard-coded.
func TestAllowedMethodsListsEveryRegisteredMethod(t *testing.T) {
	r := chi.NewRouter()
	noop := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	r.Get("/x", noop)
	r.Post("/x", noop)
	r.Delete("/x", noop)
	r.Get("/y", noop)
	if got, want := allowedMethods(r, "/x"), "GET, POST, DELETE"; got != want {
		t.Errorf("allowedMethods(/x) = %q, want %q", got, want)
	}
	if got, want := allowedMethods(r, "/y"), "GET"; got != want {
		t.Errorf("allowedMethods(/y) = %q, want %q", got, want)
	}
	if got := allowedMethods(r, "/nope"); got != "" {
		t.Errorf("allowedMethods(/nope) = %q, want empty", got)
	}
}

func serve(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, http.NoBody))
	return rec
}
