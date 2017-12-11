package server

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/iced-mocha/twitter-client/handlers"
)

type Server struct {
	Router *mux.Router
}

func New(api handlers.CoreAPI) (*Server, error) {
	s := &Server{Router: mux.NewRouter()}

	s.Router.HandleFunc("/v1/{userID}/authorize", api.Authorize).Methods(http.MethodGet)
	s.Router.HandleFunc("/v1/authorize_callback", api.AuthorizeCallback).Methods(http.MethodGet)
	s.Router.HandleFunc("/v1/posts", api.GetPosts).Methods(http.MethodGet)

	return s, nil
}
