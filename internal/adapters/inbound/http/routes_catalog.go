package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) catalogRoutes(r chi.Router) {
	r.Get("/products", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.g.SearchProducts(r.Context(), r.URL.Query())
		s.respond(w, out, err)
	})
	r.Get("/products/{id}", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.g.GetProduct(r.Context(), chi.URLParam(r, "id"))
		s.respond(w, out, err)
	})
	r.Post("/products/{id}:delivery-dates", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.g.DeliveryDates(r.Context(), chi.URLParam(r, "id"), readBody(r))
		s.respond(w, out, err)
	})
}
