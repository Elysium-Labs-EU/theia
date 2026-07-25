package apiserver

import (
	"crypto/subtle"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
)

var errUnauthorized = errors.New("unauthorized")

// Config is the narrow set of inputs the API server needs — not the whole
// CLI flag set.
type Config struct {
	Addr  string
	Token string
}

// NewServer builds the stats API's http.Server: every route requires a
// valid bearer token before reaching a handler. It does not listen — the
// caller controls the accept loop and shutdown, per Config.Addr.
func NewServer(db *sql.DB, cfg Config) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/stats", withAuth(cfg.Token, handleStats(db)))
	mux.HandleFunc("GET /api/v1/stats/paths", withAuth(cfg.Token, handlePaths(db)))
	mux.HandleFunc("GET /api/v1/stats/referrers", withAuth(cfg.Token, handleReferrers(db)))
	mux.HandleFunc("GET /api/v1/stats/status-codes", withAuth(cfg.Token, handleStatusCodes(db)))

	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

const bearerPrefix = "Bearer "

// withAuth rejects any request whose Authorization header doesn't carry the
// exact configured bearer token. Comparison is constant-time so a
// timing side-channel can't be used to guess the token byte by byte.
func withAuth(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, bearerPrefix) {
			writeError(w, http.StatusUnauthorized, errUnauthorized)
			return
		}

		got := strings.TrimPrefix(header, bearerPrefix)
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			writeError(w, http.StatusUnauthorized, errUnauthorized)
			return
		}

		next(w, r)
	}
}
