package handlers

import (
	"net/http"
)

type CoreAPI interface {
	Authorize(w http.ResponseWriter, r *http.Request)
	AuthorizeCallback(w http.ResponseWriter, r *http.Request)
	GetPosts(w http.ResponseWriter, r *http.Request)
}
