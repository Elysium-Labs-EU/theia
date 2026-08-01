package query

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Summary struct {
	Pageviews      int
	UniqueVisitors int
	BotViews       int
}

type PathStat struct {
	Path      string
	Host      string
	Pageviews int
}

type StatusStat struct {
	StatusCode int
	Count      int
}

type ReferrerStat struct {
	Referrer string
	Count    int
}

// SeriesPoint is one bucket of a time series returned by GetSeries — either
// a calendar day or an hour within a day, depending on the requested
// group_by. UniqueVisitors is only meaningful for day buckets: the schema
// tracks distinct visitors per (host, day), not per hour, so hour buckets
// always report it as 0.
type SeriesPoint struct {
	Date           string `json:"date"`
	PageViews      int    `json:"page_views"`
	UniqueVisitors int    `json:"unique_visitors"`
	BotViews       int    `json:"bot_views"`
}

func sinceFilter(since time.Time) (year, yearDay int) {
	return since.Year(), since.YearDay()
}

// dateRangeClause is the WHERE fragment for an inclusive [from, to] date
// range expressed in the (year, year_day) terms every hourly_* table is
// keyed on. It's a fixed literal (see rangeArgs for the bind values) rather
// than built at runtime, so gosec's SQL-concatenation check can see there's
// no injectable input in the query string.
const dateRangeClause = "((year > ? OR (year = ? AND year_day >= ?)) AND (year < ? OR (year = ? AND year_day <= ?)))"

// hostFilterClause is the WHERE fragment appended to every query below when
// a host filter is requested.
const hostFilterClause = " AND host = ?"

// rangeArgs returns the bind args for dateRangeClause over [from, to].
func rangeArgs(from, to time.Time) []any {
	fromYear, fromDay := from.Year(), from.YearDay()
	toYear, toDay := to.Year(), to.YearDay()
	return []any{fromYear, fromYear, fromDay, toYear, toYear, toDay}
}

func GetSummary(ctx context.Context, db *sql.DB, since time.Time, host string) (Summary, error) {
	year, yearDay := sinceFilter(since)

	q := `
	SELECT
		COALESCE(SUM(page_views), 0),
		COALESCE(SUM(bot_views), 0)
	FROM hourly_stats
	WHERE (year > ? OR (year = ? AND year_day >= ?))`

	args := []any{year, year, yearDay}
	if host != "" {
		q += hostFilterClause
		args = append(args, host)
	}

	var s Summary
	if err := db.QueryRowContext(ctx, q, args...).Scan(&s.Pageviews, &s.BotViews); err != nil {
		return Summary{}, fmt.Errorf("querying summary: %w", err)
	}

	uniqueVisitors, err := getUniqueVisitors(ctx, db, year, yearDay, host)
	if err != nil {
		return Summary{}, err
	}
	s.UniqueVisitors = uniqueVisitors

	return s, nil
}

func getUniqueVisitors(ctx context.Context, db *sql.DB, year, yearDay int, host string) (int, error) {
	q := `
	SELECT COUNT(DISTINCT hash)
	FROM visitor_days
	WHERE (year > ? OR (year = ? AND year_day >= ?))`

	args := []any{year, year, yearDay}
	if host != "" {
		q += hostFilterClause
		args = append(args, host)
	}

	var count int
	if err := db.QueryRowContext(ctx, q, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("querying unique visitors: %w", err)
	}
	return count, nil
}

func GetTopPaths(ctx context.Context, db *sql.DB, since time.Time, host string, limit int) ([]PathStat, error) {
	year, yearDay := sinceFilter(since)

	q := `
	SELECT path, host, SUM(page_views) as total_pv
	FROM hourly_stats
	WHERE (year > ? OR (year = ? AND year_day >= ?))
	  AND is_static = 0`

	args := []any{year, year, yearDay}
	if host != "" {
		q += hostFilterClause
		args = append(args, host)
	}
	q += " GROUP BY path, host ORDER BY total_pv DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying top paths: %w", err)
	}
	defer rows.Close() //nolint:errcheck // close error in defer is not actionable

	results := []PathStat{}
	for rows.Next() {
		var p PathStat
		if err := rows.Scan(&p.Path, &p.Host, &p.Pageviews); err != nil {
			return nil, fmt.Errorf("scanning path stat: %w", err)
		}
		results = append(results, p)
	}
	return results, rows.Err()
}

func GetStatusCodes(ctx context.Context, db *sql.DB, since time.Time, host string) ([]StatusStat, error) {
	year, yearDay := sinceFilter(since)

	q := `
	SELECT status_code, SUM(count) as total
	FROM hourly_status_codes
	WHERE (year > ? OR (year = ? AND year_day >= ?))`

	args := []any{year, year, yearDay}
	if host != "" {
		q += hostFilterClause
		args = append(args, host)
	}
	q += " GROUP BY status_code ORDER BY total DESC"

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying status codes: %w", err)
	}
	defer rows.Close() //nolint:errcheck // close error in defer is not actionable

	results := []StatusStat{}
	for rows.Next() {
		var s StatusStat
		if err := rows.Scan(&s.StatusCode, &s.Count); err != nil {
			return nil, fmt.Errorf("scanning status stat: %w", err)
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

func GetTopReferrers(ctx context.Context, db *sql.DB, since time.Time, host string, limit int) ([]ReferrerStat, error) {
	year, yearDay := sinceFilter(since)

	q := `
	SELECT referrer, SUM(count) as total
	FROM hourly_referrers
	WHERE (year > ? OR (year = ? AND year_day >= ?))
	  AND referrer != '-'`

	args := []any{year, year, yearDay}
	if host != "" {
		q += hostFilterClause
		args = append(args, host)
	}
	q += " GROUP BY referrer ORDER BY total DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying top referrers: %w", err)
	}
	defer rows.Close() //nolint:errcheck // close error in defer is not actionable

	results := []ReferrerStat{}
	for rows.Next() {
		var r ReferrerStat
		if err := rows.Scan(&r.Referrer, &r.Count); err != nil {
			return nil, fmt.Errorf("scanning referrer stat: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// GetSeries returns page view/bot view/unique visitor totals bucketed by
// calendar day or by hour over [from, to], optionally filtered by host.
// groupBy must be "day" or "hour".
func GetSeries(ctx context.Context, db *sql.DB, from, to time.Time, host, groupBy string) ([]SeriesPoint, error) {
	switch groupBy {
	case "hour":
		return getSeriesByHour(ctx, db, from, to, host)
	case "day", "":
		return getSeriesByDay(ctx, db, from, to, host)
	default:
		return nil, fmt.Errorf("invalid group_by %q: must be \"day\" or \"hour\"", groupBy)
	}
}

func getSeriesByDay(ctx context.Context, db *sql.DB, from, to time.Time, host string) ([]SeriesPoint, error) {
	args := rangeArgs(from, to)

	q := `
	SELECT year, year_day, COALESCE(SUM(page_views), 0), COALESCE(SUM(bot_views), 0)
	FROM hourly_stats
	WHERE `
	q += dateRangeClause
	if host != "" {
		q += hostFilterClause
		args = append(args, host)
	}
	q += " GROUP BY year, year_day ORDER BY year, year_day"

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying daily series: %w", err)
	}
	defer rows.Close() //nolint:errcheck // close error in defer is not actionable

	type dayTotals struct {
		year, yearDay, pageViews, botViews int
	}
	days := []dayTotals{}
	for rows.Next() {
		var d dayTotals
		if scanErr := rows.Scan(&d.year, &d.yearDay, &d.pageViews, &d.botViews); scanErr != nil {
			return nil, fmt.Errorf("scanning daily series: %w", scanErr)
		}
		days = append(days, d)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}

	visitors, err := getUniqueVisitorsByDay(ctx, db, from, to, host)
	if err != nil {
		return nil, err
	}

	results := make([]SeriesPoint, 0, len(days))
	for _, d := range days {
		results = append(results, SeriesPoint{
			Date:           yearDayToDate(d.year, d.yearDay),
			PageViews:      d.pageViews,
			UniqueVisitors: visitors[dayKey{d.year, d.yearDay}],
			BotViews:       d.botViews,
		})
	}
	return results, nil
}

type dayKey struct {
	year, yearDay int
}

func getUniqueVisitorsByDay(ctx context.Context, db *sql.DB, from, to time.Time, host string) (map[dayKey]int, error) {
	args := rangeArgs(from, to)

	q := `
	SELECT year, year_day, COUNT(DISTINCT hash)
	FROM visitor_days
	WHERE `
	q += dateRangeClause
	if host != "" {
		q += hostFilterClause
		args = append(args, host)
	}
	q += " GROUP BY year, year_day"

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying unique visitors by day: %w", err)
	}
	defer rows.Close() //nolint:errcheck // close error in defer is not actionable

	visitors := map[dayKey]int{}
	for rows.Next() {
		var k dayKey
		var count int
		if err := rows.Scan(&k.year, &k.yearDay, &count); err != nil {
			return nil, fmt.Errorf("scanning unique visitors by day: %w", err)
		}
		visitors[k] = count
	}
	return visitors, rows.Err()
}

func getSeriesByHour(ctx context.Context, db *sql.DB, from, to time.Time, host string) ([]SeriesPoint, error) {
	args := rangeArgs(from, to)

	q := `
	SELECT year, year_day, hour, COALESCE(SUM(page_views), 0), COALESCE(SUM(bot_views), 0)
	FROM hourly_stats
	WHERE `
	q += dateRangeClause
	if host != "" {
		q += hostFilterClause
		args = append(args, host)
	}
	q += " GROUP BY year, year_day, hour ORDER BY year, year_day, hour"

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying hourly series: %w", err)
	}
	defer rows.Close() //nolint:errcheck // close error in defer is not actionable

	results := []SeriesPoint{}
	for rows.Next() {
		var year, yearDay, hour, pageViews, botViews int
		if err := rows.Scan(&year, &yearDay, &hour, &pageViews, &botViews); err != nil {
			return nil, fmt.Errorf("scanning hourly series: %w", err)
		}
		results = append(results, SeriesPoint{
			Date:      fmt.Sprintf("%sT%02d:00:00", yearDayToDate(year, yearDay), hour),
			PageViews: pageViews,
			BotViews:  botViews,
		})
	}
	return results, rows.Err()
}

func yearDayToDate(year, yearDay int) string {
	return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, yearDay-1).Format("2006-01-02")
}

// GetTopPathsRange is GetTopPaths over an explicit [from, to] range instead
// of an open-ended "since now" window.
func GetTopPathsRange(ctx context.Context, db *sql.DB, from, to time.Time, host string, limit int) ([]PathStat, error) {
	args := rangeArgs(from, to)

	q := `
	SELECT path, host, SUM(page_views) as total_pv
	FROM hourly_stats
	WHERE `
	q += dateRangeClause
	q += `
	  AND is_static = 0`
	if host != "" {
		q += hostFilterClause
		args = append(args, host)
	}
	q += " GROUP BY path, host ORDER BY total_pv DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying top paths: %w", err)
	}
	defer rows.Close() //nolint:errcheck // close error in defer is not actionable

	results := []PathStat{}
	for rows.Next() {
		var p PathStat
		if err := rows.Scan(&p.Path, &p.Host, &p.Pageviews); err != nil {
			return nil, fmt.Errorf("scanning path stat: %w", err)
		}
		results = append(results, p)
	}
	return results, rows.Err()
}

// GetStatusCodesRange is GetStatusCodes over an explicit [from, to] range
// instead of an open-ended "since now" window.
func GetStatusCodesRange(ctx context.Context, db *sql.DB, from, to time.Time, host string) ([]StatusStat, error) {
	args := rangeArgs(from, to)

	q := `
	SELECT status_code, SUM(count) as total
	FROM hourly_status_codes
	WHERE `
	q += dateRangeClause
	if host != "" {
		q += hostFilterClause
		args = append(args, host)
	}
	q += " GROUP BY status_code ORDER BY total DESC"

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying status codes: %w", err)
	}
	defer rows.Close() //nolint:errcheck // close error in defer is not actionable

	results := []StatusStat{}
	for rows.Next() {
		var s StatusStat
		if err := rows.Scan(&s.StatusCode, &s.Count); err != nil {
			return nil, fmt.Errorf("scanning status stat: %w", err)
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

// GetTopReferrersRange is GetTopReferrers over an explicit [from, to] range
// instead of an open-ended "since now" window.
func GetTopReferrersRange(ctx context.Context, db *sql.DB, from, to time.Time, host string, limit int) ([]ReferrerStat, error) {
	args := rangeArgs(from, to)

	q := `
	SELECT referrer, SUM(count) as total
	FROM hourly_referrers
	WHERE `
	q += dateRangeClause
	q += `
	  AND referrer != '-'`
	if host != "" {
		q += hostFilterClause
		args = append(args, host)
	}
	q += " GROUP BY referrer ORDER BY total DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying top referrers: %w", err)
	}
	defer rows.Close() //nolint:errcheck // close error in defer is not actionable

	results := []ReferrerStat{}
	for rows.Next() {
		var r ReferrerStat
		if err := rows.Scan(&r.Referrer, &r.Count); err != nil {
			return nil, fmt.Errorf("scanning referrer stat: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
