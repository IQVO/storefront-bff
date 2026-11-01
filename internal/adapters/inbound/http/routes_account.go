package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) accountRoutes(r chi.Router) {
	r.Get("/orders", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.g.ListOrders(r.Context(), actor(r), r.URL.Query())
		s.respond(w, out, err)
	})
	r.Get("/orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.g.GetOrder(r.Context(), actor(r), chi.URLParam(r, "id"))
		s.respond(w, out, err)
	})
	r.Post("/orders/{id}:cancel", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.g.CancelOrder(r.Context(), actor(r), chi.URLParam(r, "id"), readBody(r))
		s.respond(w, out, err)
	})
	r.Post("/shoppers", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.g.RegisterShopper(r.Context(), actor(r), readBody(r))
		s.respond(w, out, err)
	})
	r.Post("/sessions", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.g.Login(r.Context(), actor(r), readBody(r))
		s.respond(w, out, err)
	})
	r.Get("/me", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.g.Me(r.Context(), actor(r))
		s.respond(w, out, err)
	})
	r.Post("/me/addresses", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.g.AddAddress(r.Context(), actor(r), readBody(r))
		s.respond(w, out, err)
	})
}
