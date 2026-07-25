package apiserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// shutdownGrace bounds how long Run waits for in-flight requests to finish
// once ctx is canceled before forcing the listener closed.
const shutdownGrace = 5 * time.Second

// Run starts the stats API server and blocks until ctx is canceled or the
// server fails to start, then shuts it down gracefully.
func Run(ctx context.Context, db *sql.DB, cfg Config) error {
	srv := NewServer(db, cfg)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("stats API server: %w", err)
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
			return fmt.Errorf("shutting down stats API server: %w", err)
		}
		return nil
	}
}

// ValidateLoopbackAddr rejects any address whose host isn't 127.0.0.1 or
// localhost — theia's stats API must never be reachable except through an
// explicit reverse proxy on the same machine.
func ValidateLoopbackAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid --addr %q: %w", addr, err)
	}
	if host != "127.0.0.1" && host != "localhost" {
		return fmt.Errorf("invalid --addr %q: must bind to 127.0.0.1 or localhost only", addr)
	}
	return nil
}
