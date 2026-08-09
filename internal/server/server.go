// Package server wires the HTTP routes and handlers for the service.
package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	onklave "github.com/onklave/onklave-go"
)

// New returns an http.Handler with all routes registered. Keeping the mux
// construction here (separate from main) makes the handlers testable without
// starting a real server. errs may be a no-op (key-less or nil) client —
// capture calls are then silently skipped.
func New(logger *slog.Logger, errs *onklave.Client) http.Handler {
	mux := http.NewServeMux()

	// Go 1.22 enhanced routing: method + path patterns. The "{$}" anchor makes
	// "GET /{$}" match the root path exactly, so unknown paths fall through to
	// a 404 instead of being swallowed by a "GET /" subtree match.
	mux.HandleFunc("GET /healthz", handleHealthz())
	mux.HandleFunc("GET /{$}", handleRoot())

	return recoverPanics(logger, errs, logRequests(logger, mux))
}

// handleHealthz reports service liveness. Used by Onklave for health checks.
func handleHealthz() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// handleRoot returns a small JSON greeting. It is registered under "GET /{$}"
// so only the exact root path matches; unmatched paths fall through to a 404.
func handleRoot() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"service": "template-go-web-service",
			"message": "hello from Onklave",
		})
	}
}

// recoverPanics is the central error path: it converts a handler panic into
// a 500 response and reports it to Onklave error tracking (fire-and-forget;
// a no-op client is fine).
func recoverPanics(logger *slog.Logger, errs *onklave.Client, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			err, ok := rec.(error)
			if !ok {
				err = fmt.Errorf("panic: %v", rec)
			}
			logger.Error("handler panicked",
				slog.Any("error", err),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
			)
			errs.CaptureException(err, onklave.WithRequest(onklave.RequestInfo{
				Method:     r.Method,
				Path:       r.URL.Path,
				StatusCode: http.StatusInternalServerError,
			}))
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
		}()
		next.ServeHTTP(w, r)
	})
}

// logRequests is a tiny middleware logging each request via slog.
func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info("request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("remote", r.RemoteAddr),
		)
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Encoding a simple map never errors; ignore for brevity.
	_ = json.NewEncoder(w).Encode(body)
}
