package http

import (
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/IQVO/storefront-bff/internal/application/usecases"
	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

// BasePath is where the gateway mounts the storefront edge. The prefix is
// optional on the wire so the BFF also answers behind a path-stripping proxy.
const BasePath = "/api/storefront"

// Server serves the storefront edge.
type Server struct {
	g      *usecases.Gateway
	log    *slog.Logger
	secure bool
}

// NewServer builds the edge. secure sets the Secure attribute of the cookie.
func NewServer(g *usecases.Gateway, log *slog.Logger, secure bool) *Server {
	return &Server{g: g, log: log, secure: secure}
}

// Handler returns the full router, including /healthz and /readyz.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(recoverPanic(s.log), accessLog(s.log), rejectNUL)
	r.Get("/healthz", ok)
	r.Get("/readyz", ok)
	s.catalogRoutes(r)
	s.cartRoutes(r)
	s.checkoutRoutes(r)
	s.accountRoutes(r)
	r.NotFound(func(w http.ResponseWriter, _ *http.Request) { writeProblem(w, pNotFound, "") })
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeProblem(w, problemKind{http.StatusMethodNotAllowed, "method-not-allowed", "Method not allowed"}, "")
	})
	return stripBase(r)
}

func stripBase(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p, found := strings.CutPrefix(r.URL.Path, BasePath); found && (p == "" || p[0] == '/') {
			r2 := r.Clone(r.Context())
			r2.URL.Path = p
			if r2.URL.Path == "" {
				r2.URL.Path = "/"
			}
			r2.URL.RawPath = ""
			r = r2
		}
		next.ServeHTTP(w, r)
	})
}

func ok(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"status":"ok"}`)
}

// actor reads the cookie state and the page's optional action id.
func actor(r *http.Request) usecases.Actor {
	a := usecases.Actor{ActionID: r.Header.Get("X-Action-Id")}
	if c, err := r.Cookie(composition.CookieName); err == nil {
		a.State = composition.DecodeState(c.Value)
	}
	return a
}

// respond relays a use case outcome: the cookie when the state changed, then
// the upstream answer (status, content type, body) untouched.
func (s *Server) respond(w http.ResponseWriter, out usecases.Outcome, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	if out.SetState {
		http.SetCookie(w, s.cookie(out.State))
	}
	ct := out.Reply.ContentType
	if ct == "" {
		ct = "application/json"
	}
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(out.Reply.Status)
	_, _ = w.Write(out.Reply.Body)
}

// cookie is state, not authentication: HttpOnly, SameSite=Lax, scoped to the
// storefront path, valid for 30 days.
func (s *Server) cookie(st composition.State) *http.Cookie {
	//nolint:gosec // Secure follows COOKIE_SECURE so plain-http local runs still carry the state cookie
	return &http.Cookie{
		Name: composition.CookieName, Value: st.Encode(), Path: BasePath,
		MaxAge: 30 * 24 * 3600, HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode,
	}
}

func readBody(r *http.Request) []byte {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil
	}
	return b
}
