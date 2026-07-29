package promsink

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// shutdownGrace bounds how long Run waits for in-flight requests to finish
// once ctx is canceled before forcing the listener closed.
const shutdownGrace = 5 * time.Second

// NewServer builds the metrics endpoint's http.Server. It does not listen —
// the caller controls the accept loop and shutdown, per Config.Addr.
func NewServer(db *sql.DB, cfg Config) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics", Handler(db, cfg.Top))

	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

// Run starts the metrics server and blocks until ctx is canceled or the
// server fails to start, then shuts it down gracefully.
func Run(ctx context.Context, db *sql.DB, cfg Config) error {
	srv := NewServer(db, cfg)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("metrics server: %w", err)
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		// Draining in-flight requests must not be aborted by the same
		// cancellation that signals shutdown, so Shutdown uses a fresh
		// timeout derived from a context that keeps values but drops the
		// cancel signal.
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownGrace)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down metrics server: %w", err)
		}
		return nil
	}
}
