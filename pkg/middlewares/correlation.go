package middlewares

import (
	"net/http"
	"regexp"

	"github.com/gorilla/mux"
)

var correlationIDRegexp = regexp.MustCompile(`^[\w-_]{3,25}$`)

type CorrelationMw struct {
	headerName string
	next       http.Handler
}

func NewCorrelationMw(headerName string) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return NewCorrelation(headerName, next)
	}
}

func NewCorrelation(headerName string, next http.Handler) *CorrelationMw {
	return &CorrelationMw{headerName: headerName, next: next}
}

func (mw *CorrelationMw) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	// Copy the X-Correliation-Id header from the request to the response
	id, ok := mw.validateID(r)
	if ok {
		rw.Header().Set(mw.headerName, id)
	}

	mw.next.ServeHTTP(rw, r)
}

func (mw *CorrelationMw) validateID(r *http.Request) (string, bool) {
	hn := http.CanonicalHeaderKey(mw.headerName)
	ids, ok := r.Header[hn]

	// Validate the ID if it was supplied
	if ok {
		id := ids[0]
		if correlationIDRegexp.MatchString(id) {
			return id, true
		}

		return "<Bad_Correlation_Id>", true
	}

	return "", false
}
