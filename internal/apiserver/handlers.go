package apiserver

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Elysium-Labs-EU/theia/internal/query"
)

type dateRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type statsResponse struct {
	Host    string              `json:"host"`
	Range   dateRange           `json:"range"`
	GroupBy string              `json:"group_by"`
	Series  []query.SeriesPoint `json:"series"`
}

type pathEntry struct {
	Path      string `json:"path"`
	Host      string `json:"host"`
	PageViews int    `json:"page_views"`
}

type pathsResponse struct {
	Host  string      `json:"host"`
	Range dateRange   `json:"range"`
	Paths []pathEntry `json:"paths"`
}

type referrerEntry struct {
	Referrer string `json:"referrer"`
	Count    int    `json:"count"`
}

type referrersResponse struct {
	Host      string          `json:"host"`
	Range     dateRange       `json:"range"`
	Referrers []referrerEntry `json:"referrers"`
}

type statusCodeEntry struct {
	StatusCode int `json:"status_code"`
	Count      int `json:"count"`
}

type statusCodesResponse struct {
	Host        string            `json:"host"`
	Range       dateRange         `json:"range"`
	StatusCodes []statusCodeEntry `json:"status_codes"`
}

func handleStats(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params, err := parseStatsParams(r.URL.Query())
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		series, err := query.GetSeries(r.Context(), db, params.From, params.To, params.Host, params.GroupBy)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		if params.Format == "csv" {
			writeSeriesCSV(w, series)
			return
		}
		writeJSON(w, statsResponse{
			Host:    params.Host,
			Range:   dateRange{From: params.From.Format(dateLayout), To: params.To.Format(dateLayout)},
			GroupBy: params.GroupBy,
			Series:  series,
		})
	}
}

func handlePaths(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params, err := parseBreakdownParams(r.URL.Query())
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		stats, err := query.GetTopPathsRange(r.Context(), db, params.From, params.To, params.Host, params.Top)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		entries := make([]pathEntry, 0, len(stats))
		for _, s := range stats {
			entries = append(entries, pathEntry{Path: s.Path, Host: s.Host, PageViews: s.Pageviews})
		}

		if params.Format == "csv" {
			writePathsCSV(w, entries)
			return
		}
		writeJSON(w, pathsResponse{
			Host:  params.Host,
			Range: dateRange{From: params.From.Format(dateLayout), To: params.To.Format(dateLayout)},
			Paths: entries,
		})
	}
}

func handleReferrers(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params, err := parseBreakdownParams(r.URL.Query())
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		stats, err := query.GetTopReferrersRange(r.Context(), db, params.From, params.To, params.Host, params.Top)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		entries := make([]referrerEntry, 0, len(stats))
		for _, s := range stats {
			entries = append(entries, referrerEntry{Referrer: s.Referrer, Count: s.Count})
		}

		if params.Format == "csv" {
			writeReferrersCSV(w, entries)
			return
		}
		writeJSON(w, referrersResponse{
			Host:      params.Host,
			Range:     dateRange{From: params.From.Format(dateLayout), To: params.To.Format(dateLayout)},
			Referrers: entries,
		})
	}
}

func handleStatusCodes(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params, err := parseBreakdownParams(r.URL.Query())
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		stats, err := query.GetStatusCodesRange(r.Context(), db, params.From, params.To, params.Host)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		entries := make([]statusCodeEntry, 0, len(stats))
		for _, s := range stats {
			entries = append(entries, statusCodeEntry{StatusCode: s.StatusCode, Count: s.Count})
		}

		if params.Format == "csv" {
			writeStatusCodesCSV(w, entries)
			return
		}
		writeJSON(w, statusCodesResponse{
			Host:        params.Host,
			Range:       dateRange{From: params.From.Format(dateLayout), To: params.To.Format(dateLayout)},
			StatusCodes: entries,
		})
	}
}

const contentTypeHeader = "Content-Type"

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set(contentTypeHeader, "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set(contentTypeHeader, "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func writeSeriesCSV(w http.ResponseWriter, series []query.SeriesPoint) {
	cw := newCSVWriter(w)
	_ = cw.Write([]string{"date", "page_views", "unique_visitors", "bot_views"})
	for _, s := range series {
		_ = cw.Write([]string{
			s.Date,
			strconv.Itoa(s.PageViews),
			strconv.Itoa(s.UniqueVisitors),
			strconv.Itoa(s.BotViews),
		})
	}
	cw.Flush()
}

func writePathsCSV(w http.ResponseWriter, entries []pathEntry) {
	cw := newCSVWriter(w)
	_ = cw.Write([]string{"path", "host", "page_views"})
	for _, e := range entries {
		_ = cw.Write([]string{e.Path, e.Host, strconv.Itoa(e.PageViews)})
	}
	cw.Flush()
}

func writeReferrersCSV(w http.ResponseWriter, entries []referrerEntry) {
	cw := newCSVWriter(w)
	_ = cw.Write([]string{"referrer", "count"})
	for _, e := range entries {
		_ = cw.Write([]string{e.Referrer, strconv.Itoa(e.Count)})
	}
	cw.Flush()
}

func writeStatusCodesCSV(w http.ResponseWriter, entries []statusCodeEntry) {
	cw := newCSVWriter(w)
	_ = cw.Write([]string{"status_code", "count"})
	for _, e := range entries {
		_ = cw.Write([]string{strconv.Itoa(e.StatusCode), strconv.Itoa(e.Count)})
	}
	cw.Flush()
}

// newCSVWriter sets the CSV content type and returns a writer over w. Writes
// are best-effort: a client that disconnects mid-stream isn't actionable,
// and csv.Writer surfaces that same error again from Flush/Error if it
// matters, which callers are not expected to check for by design.
func newCSVWriter(w http.ResponseWriter) *csv.Writer {
	w.Header().Set(contentTypeHeader, "text/csv; charset=utf-8")
	return csv.NewWriter(w)
}
