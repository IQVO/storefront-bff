package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/IQVO/storefront-bff/internal/application/usecases"
)

// cartCmd adapts a cart command that takes (actor, cartId, body).
func (s *Server) cartCmd(f func(*usecases.Gateway, *http.Request, usecases.Actor, string, []byte) (usecases.Outcome, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := f(s.g, r, actor(r), chi.URLParam(r, "id"), readBody(r))
		s.respond(w, out, err)
	}
}

func (s *Server) cartRoutes(r chi.Router) {
	r.Post("/carts", requireKey(func(w http.ResponseWriter, r *http.Request) {
		out, err := s.g.CreateCart(r.Context(), actor(r), readBody(r))
		s.respond(w, out, err)
	}))
	r.Get("/carts", func(w http.ResponseWriter, r *http.Request) {
		if !pageParams(w, r, "shopperId", "state") {
			return
		}
		out, err := s.g.ListCarts(r.Context(), actor(r), r.URL.Query())
		s.respond(w, out, err)
	})
	r.Get("/carts/{id}", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.g.GetCart(r.Context(), chi.URLParam(r, "id"))
		s.respond(w, out, err)
	})
	r.Post("/carts/{id}/lines", requireKey(s.cartCmd(func(g *usecases.Gateway, r *http.Request, a usecases.Actor, id string, b []byte) (usecases.Outcome, error) {
		return g.AddLine(r.Context(), a, id, b)
	})))
	r.Put("/carts/{id}/destination", s.cartCmd(func(g *usecases.Gateway, r *http.Request, a usecases.Actor, id string, b []byte) (usecases.Outcome, error) {
		return g.SetDestination(r.Context(), a, id, b)
	}))
	r.Post("/carts/{id}/delivery-quotes", requireKey(s.cartCmd(func(g *usecases.Gateway, r *http.Request, a usecases.Actor, id string, _ []byte) (usecases.Outcome, error) {
		return g.QuoteDelivery(r.Context(), a, id)
	})))
	r.Put("/carts/{id}/delivery-selection", s.cartCmd(func(g *usecases.Gateway, r *http.Request, a usecases.Actor, id string, b []byte) (usecases.Outcome, error) {
		return g.SelectDelivery(r.Context(), a, id, b)
	}))
	r.Patch("/carts/{id}/lines/{item}", requireKey(func(w http.ResponseWriter, r *http.Request) {
		out, err := s.g.SetLineQuantity(r.Context(), actor(r), chi.URLParam(r, "id"), chi.URLParam(r, "item"), readBody(r))
		s.respond(w, out, err)
	}))
	r.Delete("/carts/{id}/lines/{item}", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.g.RemoveLine(r.Context(), actor(r), chi.URLParam(r, "id"), chi.URLParam(r, "item"))
		s.respond(w, out, err)
	})
}
