package api

import (
	"log/slog"
	"net/http"
	"time"
)

// middleware is the standard http.Handler decorator signature. We compose a few
// of these by hand instead of pulling in a framework — each is a dozen lines and
// visible, which suits a learning codebase (see ADR-0005).
type middleware func(http.Handler) http.Handler

// statusRecorder captures the status code a handler writes so the logging
// middleware can report it (ResponseWriter doesn't expose it otherwise).
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// logMW logs one line per request: method, path, status, and duration.
func logMW(log *slog.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			log.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

// recoverMW turns a panic in any handler into a 500 instead of crashing the
// process, logging the panic value for debugging.
func recoverMW(log *slog.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					log.Error("panic recovered", "value", v, "path", r.URL.Path)
					writeError(w, http.StatusInternalServerError, "internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// corsMW allows the dashboard (and SDK from a browser) to call the API. It's
// permissive by design for a dev tool; a production deployment would restrict the
// allowed origin.
func corsMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
