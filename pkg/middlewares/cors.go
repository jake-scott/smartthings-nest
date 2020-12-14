package middlewares

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
)

type CorsMw struct {
	h http.Handler
}

func NewCorsMw(opts cors.Options) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return NewCors(opts, next)
	}
}

// Called once for each middleware chain
//
func NewCors(opts cors.Options, next http.Handler) *CorsMw {
	cors := cors.New(opts)

	return &CorsMw{
		h: cors.Handler(next),
	}
}

// This should be the first Middleware in the chain
//
func (mw *CorsMw) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	fmt.Printf("In CorsMw::ServeHTTP: PRE\n")

	mw.h.ServeHTTP(rw, r)

	fmt.Printf("In CorsMw::ServeHTTP : POST\n")
}
