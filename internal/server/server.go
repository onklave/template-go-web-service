// Package server wires the HTTP routes and handlers for the service.
package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// New returns an http.Handler with all routes registered. Keeping the mux
// construction here (separate from main) makes the handlers testable without
// starting a real server.
func New(logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	// Go 1.22 enhanced routing: method + path patterns. The "{$}" anchor makes
	// "GET /{$}" match the root path exactly, so unknown paths fall through to
	// a 404 instead of being swallowed by a "GET /" subtree match.
	mux.HandleFunc("GET /healthz", handleHealthz())
	mux.HandleFunc("GET /{$}", handleRoot())

	return logRequests(logger, mux)
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
