package core

import "net/http"

// Router is how modules attach HTTP/WS routes.
type Router interface {
	Register(pattern string, handler http.HandlerFunc)
}
