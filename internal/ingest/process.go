package ingest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func processPageviews(ctx context.Context, db *sql.DB, pageViews <-chan PageView) {
	for {
		pageView, ok := <-pageViews

		if !ok {
			break
		}

		visitorHashesQuery := `
		SELECT hash
		FROM visitor_hashes
		WHERE hash = ?`

		row := db.QueryRowContext(ctx, visitorHashesQuery, pageView.IDHash)

		var visitorHash string
		err := row.Scan(&visitorHash)
		if errors.Is(err, sql.ErrNoRows) {
			visitorHashesUpdateQuery := `
			INSERT INTO visitor_hashes (hash, hour_bucket, first_seen)
			VALUES (?, ?, ?)
			`

			_, err = db.ExecContext(ctx, visitorHashesUpdateQuery, pageView.IDHash, pageView.Timestamp.Hour(), pageView.Timestamp.Format("2006-01-02 15:04:05"))
			if err != nil {
				fmt.Printf("Unable to write visitor hash into database, got: %v\n", err)
			}
		} else if err != nil {
			fmt.Printf("Unable to parse visitor hash from database, got: %v\n", err)
		}

		visitorHourBucketQuery := `
		SELECT COUNT(hash)
		FROM visitor_hashes
		WHERE hour_bucket = ?`

		visitorHashesCount := db.QueryRowContext(ctx, visitorHourBucketQuery, pageView.Timestamp.Hour())
		var visitorHashesCountResult int

		err = visitorHashesCount.Scan(&visitorHashesCountResult)
		if errors.Is(err, sql.ErrNoRows) {
			fmt.Print("No visitors found, should have at least one visitor at this point")
		} else if err != nil {
			fmt.Printf("Unable to retrieve count of visitor hash from database, got: %v\n", err)
		}

		hourlyStatsUpdateQuery := `
		INSERT INTO hourly_stats (hour, year_day, year, path, host, page_views, is_static, unique_visitors, bot_views)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(hour, year_day, year, path, host) DO UPDATE SET
			page_views = page_views + ?,
			unique_visitors = ?,
			bot_views = bot_views + ?
		`

		pageViewIncrement := 0
		botViewIncrement := 0
		if pageView.IsBot {
			botViewIncrement = 1
		} else {
			pageViewIncrement = 1
		}

		_, err = db.ExecContext(ctx, hourlyStatsUpdateQuery,
			pageView.Timestamp.Hour(),
			pageView.Timestamp.YearDay(),
			pageView.Timestamp.Year(),
			pageView.Path,
			pageView.Host,
			pageViewIncrement,
			pageView.IsStatic,
			visitorHashesCountResult,
			botViewIncrement,
			pageViewIncrement,
			visitorHashesCountResult,
			botViewIncrement)
		if err != nil {
			fmt.Printf("Unable to write hourly stats into database, got: %v\n", err)
		}

		hourlyStatusCodesUpdateQuery := `
		INSERT INTO hourly_status_codes (hour, year_day, year, path, host, status_code, count)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(hour, year_day, year, path, host, status_code) DO UPDATE SET
			count = count + ?
		`
		_, err = db.ExecContext(ctx, hourlyStatusCodesUpdateQuery,
			pageView.Timestamp.Hour(),
			pageView.Timestamp.YearDay(),
			pageView.Timestamp.Year(),
			pageView.Path,
			pageView.Host,
			pageView.StatusCode,
			1,
			1)
		if err != nil {
			fmt.Printf("Unable to write hourly status codes into database, got: %v\n", err)
		}

		hourlyReferrersUpdateQuery := `
		INSERT INTO hourly_referrers (hour, year_day, year, path, host, referrer, count)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(hour, year_day, year, path, host, referrer) DO UPDATE SET
			count = count + ?
		`

		_, err = db.ExecContext(ctx, hourlyReferrersUpdateQuery,
			pageView.Timestamp.Hour(),
			pageView.Timestamp.YearDay(),
			pageView.Timestamp.Year(),
			pageView.Path,
			pageView.Host,
			pageView.Referrer,
			1,
			1)
		if err != nil {
			fmt.Printf("Unable to write hourly referrers into database, got: %v\n", err)
		}
	}
}

func performAllCleanups(ctx context.Context, db *sql.DB) {
	if deleted, err := dbCleanUpOldHourlyStats(ctx, db); err != nil {
		fmt.Printf("Hourly stats cleanup error: %v\n", err)
	} else {
		fmt.Printf("Cleaned up %d old hourly stats records\n", deleted)
	}

	if deleted, err := dbCleanUpOldHourlyStatusCodes(ctx, db); err != nil {
		fmt.Printf("Status codes cleanup error: %v\n", err)
	} else {
		fmt.Printf("Cleaned up %d old status code records\n", deleted)
	}

	if deleted, err := dbCleanUpOldHourlyReferrer(ctx, db); err != nil {
		fmt.Printf("Referrers cleanup error: %v\n", err)
	} else {
		fmt.Printf("Cleaned up %d old referrer records\n", deleted)
	}

	if deleted, err := dbCleanUpOldVisitorHashes(ctx, db); err != nil {
		fmt.Printf("Visitor hashes cleanup error: %v\n", err)
	} else {
		fmt.Printf("Cleaned up %d old visitor hash records\n", deleted)
	}
}

func dbCleanUpOldHourlyStats(ctx context.Context, db *sql.DB) (int64, error) {
	cutoffDate := time.Now().AddDate(0, 0, -60)
	cutoffYear := cutoffDate.Year()
	cutoffYearDay := cutoffDate.YearDay()

	query := `
	DELETE FROM hourly_stats
	WHERE year < ?
	   OR (year = ? AND year_day < ?)`

	result, err := db.ExecContext(ctx, query, cutoffYear, cutoffYear, cutoffYearDay)
	if err != nil {
		return 0, fmt.Errorf("could not delete old hourly stats records, %w", err)
	}

	rowsDeleted, _ := result.RowsAffected()
	return rowsDeleted, nil
}

func dbCleanUpOldHourlyStatusCodes(ctx context.Context, db *sql.DB) (int64, error) {
	cutoffDate := time.Now().AddDate(0, 0, -60)
	cutoffYear := cutoffDate.Year()
	cutoffYearDay := cutoffDate.YearDay()

	query := `
	DELETE FROM hourly_status_codes
	WHERE year < ?
	   OR (year = ? AND year_day < ?)`

	result, err := db.ExecContext(ctx, query, cutoffYear, cutoffYear, cutoffYearDay)
	if err != nil {
		return 0, fmt.Errorf("could not delete old hourly status codes records, %w", err)
	}

	rowsDeleted, _ := result.RowsAffected()
	return rowsDeleted, nil
}

func dbCleanUpOldHourlyReferrer(ctx context.Context, db *sql.DB) (int64, error) {
	cutoffDate := time.Now().AddDate(0, 0, -60)
	cutoffYear := cutoffDate.Year()
	cutoffYearDay := cutoffDate.YearDay()

	query := `
	DELETE FROM hourly_referrers
	WHERE year < ?
	   OR (year = ? AND year_day < ?)`

	result, err := db.ExecContext(ctx, query, cutoffYear, cutoffYear, cutoffYearDay)
	if err != nil {
		return 0, fmt.Errorf("could not delete old hourly referrer records, %w", err)
	}

	rowsDeleted, _ := result.RowsAffected()
	return rowsDeleted, nil
}

func dbCleanUpOldVisitorHashes(ctx context.Context, db *sql.DB) (int64, error) {
	query := `
	DELETE FROM visitor_hashes
	WHERE datetime(first_seen) < datetime('now', '-1 day');`

	result, err := db.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("could not delete old records, %w", err)
	}

	rowsDeleted, _ := result.RowsAffected()
	return rowsDeleted, nil
}
