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

func sinceFilter(since time.Time) (year, yearDay int) {
	return since.Year(), since.YearDay()
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
		q += " AND host = ?"
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
		q += " AND host = ?"
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
		q += " AND host = ?"
		args = append(args, host)
	}
	q += " GROUP BY path, host ORDER BY total_pv DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying top paths: %w", err)
	}
	defer rows.Close() //nolint:errcheck // close error in defer is not actionable

	var results []PathStat
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
		q += " AND host = ?"
		args = append(args, host)
	}
	q += " GROUP BY status_code ORDER BY total DESC"

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying status codes: %w", err)
	}
	defer rows.Close() //nolint:errcheck // close error in defer is not actionable

	var results []StatusStat
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
		q += " AND host = ?"
		args = append(args, host)
	}
	q += " GROUP BY referrer ORDER BY total DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying top referrers: %w", err)
	}
	defer rows.Close() //nolint:errcheck // close error in defer is not actionable

	var results []ReferrerStat
	for rows.Next() {
		var r ReferrerStat
		if err := rows.Scan(&r.Referrer, &r.Count); err != nil {
			return nil, fmt.Errorf("scanning referrer stat: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
