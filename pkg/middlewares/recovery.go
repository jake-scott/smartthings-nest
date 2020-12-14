package middlewares

import (
	"net/http"
	"runtime/debug"

	"github.com/gorilla/mux"
	"github.com/jake-scott/smartthings-nest/internal/pkg/logging"
)

type RecoveryMw struct {
	next http.Handler
}

func NewRecoveryMw() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return NewRecovery(next)
	}
}

func NewRecovery(next http.Handler) *RecoveryMw {
	return &RecoveryMw{next: next}
}

func (mw *RecoveryMw) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			logging.Logger(r.Context()).Errorf("caught panic: %v : %s", err, debug.Stack())

			http.Error(rw, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	}()

	mw.next.ServeHTTP(rw, r)
}
