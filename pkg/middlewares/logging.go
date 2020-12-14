package middlewares

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/jake-scott/smartthings-nest/internal/pkg/logging"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type responseWriterEx struct {
	http.ResponseWriter

	statusCode       int
	size             int
	logData          bool
	ctx              context.Context
	hasLoggedHeaders bool
}

func newResponseWriterEx(ctx context.Context, logData bool, rw http.ResponseWriter) responseWriterEx {
	return responseWriterEx{
		ResponseWriter: rw,
		statusCode:     http.StatusOK,
		logData:        logData,
		ctx:            ctx,
	}
}

func (rw *responseWriterEx) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

func (rw *responseWriterEx) Write(b []byte) (int, error) {
	if rw.logData && !rw.hasLoggedHeaders {
		logging.Logger(rw.ctx).Debugf("wrote headers: %+v", rw.ResponseWriter.Header())
		rw.hasLoggedHeaders = true
	}

	size, err := rw.ResponseWriter.Write(b)
	rw.size += size

	if err == nil && rw.logData {
		logging.Logger(rw.ctx).Debugf("wrote %d bytes: %s", size, b[:size])
	}
	return size, err
}

// Wrapper around an io.ReadCloser that logs every read as a string
type loggingReader struct {
	io.ReadCloser
	ctx context.Context
}

func newLoggingReader(ctx context.Context, rc io.ReadCloser) io.ReadCloser {
	return loggingReader{
		ReadCloser: rc,
		ctx:        ctx,
	}
}

func (lr loggingReader) Read(b []byte) (size int, err error) {
	size, err = lr.ReadCloser.Read(b)
	if size > 0 {
		logging.Logger(lr.ctx).Debugf("read %d bytes: --:--%s--:--", size, b[:size])
	}

	return size, err
}

type LoggingMw struct {
	logRequests bool
	next        http.Handler
}

// Called once
func NewLoggingMw(reqLogging bool) mux.MiddlewareFunc {
	// Called each requet
	return func(next http.Handler) http.Handler {
		return NewLogging(reqLogging, next)
	}
}

// Called by the router for each request
func NewLogging(reqLogging bool, next http.Handler) *LoggingMw {
	return &LoggingMw{next: next, logRequests: reqLogging}
}

func (mw *LoggingMw) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	txnID := uuid.New().String()
	startTime := time.Now()

	// Set the output header now before something writes any response body
	rw.Header().Set("X-Txn-ID", txnID)

	// Save the transaction ID to the request context for use in the logger
	r = r.WithContext(logging.WithTxnID(r.Context(), txnID))

	// Replace the Body reader with a logging wrapper if we're logging requests
	if mw.logRequests {
		logging.Logger(r.Context()).Debugf("request headers: %+v", r.Header)
		r.Body = newLoggingReader(r.Context(), r.Body)
	}

	// wrap the request writer so we can capture the status code and size
	rwex := newResponseWriterEx(r.Context(), mw.logRequests, rw)
	mw.next.ServeHTTP(&rwex, r)

	logrus.WithFields(
		logrus.Fields{
			"entrytype": "audit",
			"status":    rwex.statusCode,
			"method":    r.Method,
			"proto":     r.Proto,
			"host":      r.Host,
			"remote":    r.RemoteAddr,
			"start":     startTime.Format(time.RFC3339Nano),
			"duration":  time.Since(startTime),
			"path":      r.URL.String(),
			"txnid":     txnID,
			"size":      rwex.size,
		},
	).Info(http.StatusText(rwex.statusCode))
}
