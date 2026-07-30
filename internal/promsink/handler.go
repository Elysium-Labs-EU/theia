package promsink

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/Elysium-Labs-EU/theia/internal/query"
)

// epoch is the lower bound of the "all time" window metrics are queried
// over. Prometheus counters must only grow, so every scrape re-derives the
// same cumulative totals from theia's own data rather than a rolling
// window — using theia's query.*Range functions with an open start avoids
// a nil-range special case in the query package itself.
var epoch = time.Unix(0, 0).UTC()

// Config is the narrow set of inputs the metrics endpoint needs.
type Config struct {
	Addr string
	// Top bounds how many distinct paths/referrers are exported, so an
	// attacker-controlled or just high-cardinality access log can't turn
	// every unique path into its own Prometheus time series.
	Top int
}

// Handler builds the /metrics HTTP handler: on every scrape it re-queries
// theia's sqlite database for cumulative pageview, status-code, and
// referrer counts and renders them as Prometheus counters.
func Handler(db *sql.DB, top int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()

		paths, err := query.GetTopPathsRange(r.Context(), db, epoch, now, "", top)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		statusCodes, err := query.GetStatusCodesRange(r.Context(), db, epoch, now, "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		referrers, err := query.GetTopReferrersRange(r.Context(), db, epoch, now, "", top)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(Render(Snapshot{Paths: paths, StatusCodes: statusCodes, Referrers: referrers})))
	}
}
