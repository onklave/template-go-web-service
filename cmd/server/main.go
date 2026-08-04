// Command server starts the HTTP web service: it reads config from the
// environment, wires the router, and runs with graceful shutdown on signal.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/onklave/template-go-web-service/internal/server"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	addr := ":" + getenv("PORT", "8080")

	// Every timeout below is set deliberately. A zero-value timeout on
	// http.Server means "no deadline", which lets a client hold a connection
	// open indefinitely (slowloris, trickled bodies, idle keep-alive hoarding).
	srv := &http.Server{
		Addr:    addr,
		Handler: server.New(logger),

		// Bounds the request line + headers: the classic slowloris defence.
		ReadHeaderTimeout: 5 * time.Second,
		// Bounds headers + body, so a trickled body cannot pin a connection.
		ReadTimeout: 15 * time.Second,
		// Bounds the whole response write, so a slow reader cannot pin a
		// connection. Raise this if a handler streams or does long work.
		WriteTimeout: 30 * time.Second,
		// Bounds how long an idle keep-alive connection is retained.
		IdleTimeout: 60 * time.Second,
	}

	// Run the server in a goroutine so main can wait on signals.
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server starting", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Wait for an interrupt/terminate signal or a fatal server error.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErr:
		logger.Error("server failed", slog.Any("error", err))
		os.Exit(1)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	// Give in-flight requests a bounded window to finish.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("server stopped cleanly")
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
