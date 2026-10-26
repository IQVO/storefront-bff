package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) checkoutRoutes(r chi.Router) {
	r.Post("/checkout", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.g.OpenCheckout(r.Context(), actor(r), readBody(r))
		s.respond(w, out, err)
	})
	r.Get("/checkout/{id}", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.g.GetCheckout(r.Context(), chi.URLParam(r, "id"))
		s.respond(w, out, err)
	})
	r.Post("/checkout/{id}:confirm", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.g.ConfirmCheckout(r.Context(), actor(r), chi.URLParam(r, "id"), readBody(r))
		s.respond(w, out, err)
	})
}
