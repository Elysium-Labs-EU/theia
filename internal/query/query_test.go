package query_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"theia/database"
	"theia/internal/query"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	err := os.MkdirAll(filepath.Dir(dbPath), 0755)
	if err != nil {
		t.Fatalf("could not create test db directory: %v", err)
	}

	db, err := database.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("unable to create test database: %v", err)
	}

	if err := database.RunMigrations(db, database.MigrationsFS, database.MigrationsPath); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	return db
}

func insertHourlyStat(t *testing.T, db *sql.DB, path, host string, ts time.Time, pageViews, uniqueVisitors, botViews int, isStatic bool) {
	t.Helper()
	staticInt := 0
	if isStatic {
		staticInt = 1
	}
	_, err := db.ExecContext(t.Context(), `
		INSERT INTO hourly_stats (hour, year_day, year, path, host, page_views, is_static, bot_views)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(hour, year_day, year, path, host) DO UPDATE SET
			page_views = page_views + ?,
			bot_views = bot_views + ?`,
		ts.Hour(), ts.YearDay(), ts.Year(), path, host,
		pageViews, staticInt, botViews,
		pageViews, botViews,
	)
	if err != nil {
		t.Fatalf("insert hourly stat: %v", err)
	}

	insertDistinctVisitorDays(t, db, path, host, ts, uniqueVisitors)
}

// insertDistinctVisitorDays seeds visitor_days with N distinct hashes for the given
// host/day so that GetSummary's COUNT(DISTINCT hash) reflects the unique visitor
// count a test expects, without any single hash colliding across paths/tests.
func insertDistinctVisitorDays(t *testing.T, db *sql.DB, path, host string, ts time.Time, count int) {
	t.Helper()
	for i := range count {
		hash := fmt.Sprintf("%s|%s|%d", host, path, i)
		_, err := db.ExecContext(t.Context(), `
			INSERT INTO visitor_days (hash, host, year, year_day, first_seen)
			VALUES (?, ?, ?, ?, datetime('now'))
			ON CONFLICT(hash, host, year, year_day) DO NOTHING`,
			hash, host, ts.Year(), ts.YearDay(),
		)
		if err != nil {
			t.Fatalf("insert visitor day: %v", err)
		}
	}
}

func insertStatusCode(t *testing.T, db *sql.DB, path, host string, ts time.Time, statusCode, count int) {
	t.Helper()
	_, err := db.ExecContext(t.Context(), `
		INSERT INTO hourly_status_codes (hour, year_day, year, path, host, status_code, count)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(hour, year_day, year, path, host, status_code) DO UPDATE SET count = count + ?`,
		ts.Hour(), ts.YearDay(), ts.Year(), path, host, statusCode, count, count,
	)
	if err != nil {
		t.Fatalf("insert status code: %v", err)
	}
}

func insertReferrer(t *testing.T, db *sql.DB, path, host, referrer string, ts time.Time, count int) {
	t.Helper()
	_, err := db.ExecContext(t.Context(), `
		INSERT INTO hourly_referrers (hour, year_day, year, path, host, referrer, count)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(hour, year_day, year, path, host, referrer) DO UPDATE SET count = count + ?`,
		ts.Hour(), ts.YearDay(), ts.Year(), path, host, referrer, count, count,
	)
	if err != nil {
		t.Fatalf("insert referrer: %v", err)
	}
}

func TestGetSummary(t *testing.T) {
	db := setupTestDB(t)
	defer database.Close(db) //nolint:errcheck // close error in defer is not actionable

	ctx := context.Background()
	now := time.Now()
	old := now.AddDate(0, 0, -10)

	insertHourlyStat(t, db, "/", "example.com", now, 5, 3, 1, false)
	insertHourlyStat(t, db, "/about", "example.com", now, 3, 2, 0, false)
	insertHourlyStat(t, db, "/old", "example.com", old, 100, 50, 10, false)

	since := now.AddDate(0, 0, -7)
	got, err := query.GetSummary(ctx, db, since, "")
	if err != nil {
		t.Fatalf("GetSummary: %v", err)
	}

	if got.Pageviews != 8 {
		t.Errorf("Pageviews: got %d, want 8", got.Pageviews)
	}
	if got.UniqueVisitors != 5 {
		t.Errorf("UniqueVisitors: got %d, want 5", got.UniqueVisitors)
	}
	if got.BotViews != 1 {
		t.Errorf("BotViews: got %d, want 1", got.BotViews)
	}
}

func TestGetSummaryHostFilter(t *testing.T) {
	db := setupTestDB(t)
	defer database.Close(db) //nolint:errcheck // close error in defer is not actionable

	ctx := context.Background()
	now := time.Now()

	insertHourlyStat(t, db, "/", "example.com", now, 5, 3, 0, false)
	insertHourlyStat(t, db, "/", "other.com", now, 10, 7, 0, false)

	since := now.AddDate(0, 0, -7)
	got, err := query.GetSummary(ctx, db, since, "example.com")
	if err != nil {
		t.Fatalf("GetSummary: %v", err)
	}

	if got.Pageviews != 5 {
		t.Errorf("Pageviews: got %d, want 5", got.Pageviews)
	}
}

func TestGetSummaryEmpty(t *testing.T) {
	db := setupTestDB(t)
	defer database.Close(db) //nolint:errcheck // close error in defer is not actionable

	ctx := context.Background()
	since := time.Now().AddDate(0, 0, -7)

	got, err := query.GetSummary(ctx, db, since, "")
	if err != nil {
		t.Fatalf("GetSummary on empty db: %v", err)
	}
	if got.Pageviews != 0 || got.UniqueVisitors != 0 || got.BotViews != 0 {
		t.Errorf("expected zero summary on empty db, got %+v", got)
	}
}

func TestGetTopPaths(t *testing.T) {
	db := setupTestDB(t)
	defer database.Close(db) //nolint:errcheck // close error in defer is not actionable

	ctx := context.Background()
	now := time.Now()

	insertHourlyStat(t, db, "/about", "example.com", now, 3, 2, 0, false)
	insertHourlyStat(t, db, "/", "example.com", now, 10, 5, 0, false)
	insertHourlyStat(t, db, "/style.css", "example.com", now, 50, 20, 0, true)

	since := now.AddDate(0, 0, -7)
	paths, err := query.GetTopPaths(ctx, db, since, "", 10)
	if err != nil {
		t.Fatalf("GetTopPaths: %v", err)
	}

	if len(paths) != 2 {
		t.Fatalf("expected 2 non-static paths, got %d: %v", len(paths), paths)
	}
	if paths[0].Path != "/" {
		t.Errorf("top path: got %q, want /", paths[0].Path)
	}
	if paths[0].Pageviews != 10 {
		t.Errorf("top path pageviews: got %d, want 10", paths[0].Pageviews)
	}
}

func TestGetTopPathsLimit(t *testing.T) {
	db := setupTestDB(t)
	defer database.Close(db) //nolint:errcheck // close error in defer is not actionable

	ctx := context.Background()
	now := time.Now()

	for i := range 5 {
		insertHourlyStat(t, db, fmt.Sprintf("/page%d", i), "example.com", now, i+1, 1, 0, false)
	}

	since := now.AddDate(0, 0, -7)
	paths, err := query.GetTopPaths(ctx, db, since, "", 3)
	if err != nil {
		t.Fatalf("GetTopPaths: %v", err)
	}
	if len(paths) != 3 {
		t.Errorf("expected 3 paths (limit), got %d", len(paths))
	}
}

func TestGetStatusCodes(t *testing.T) {
	db := setupTestDB(t)
	defer database.Close(db) //nolint:errcheck // close error in defer is not actionable

	ctx := context.Background()
	now := time.Now()

	insertStatusCode(t, db, "/", "example.com", now, 200, 100)
	insertStatusCode(t, db, "/missing", "example.com", now, 404, 5)
	insertStatusCode(t, db, "/err", "example.com", now, 500, 2)

	since := now.AddDate(0, 0, -7)
	codes, err := query.GetStatusCodes(ctx, db, since, "")
	if err != nil {
		t.Fatalf("GetStatusCodes: %v", err)
	}

	if len(codes) != 3 {
		t.Fatalf("expected 3 status codes, got %d", len(codes))
	}
	if codes[0].StatusCode != 200 || codes[0].Count != 100 {
		t.Errorf("first status code: got %+v, want {200, 100}", codes[0])
	}
}

func TestGetTopReferrers(t *testing.T) {
	db := setupTestDB(t)
	defer database.Close(db) //nolint:errcheck // close error in defer is not actionable

	ctx := context.Background()
	now := time.Now()

	insertReferrer(t, db, "/", "example.com", "https://google.com", now, 50)
	insertReferrer(t, db, "/", "example.com", "-", now, 200)
	insertReferrer(t, db, "/", "example.com", "https://hn.com", now, 30)

	since := now.AddDate(0, 0, -7)
	refs, err := query.GetTopReferrers(ctx, db, since, "", 10)
	if err != nil {
		t.Fatalf("GetTopReferrers: %v", err)
	}

	for _, r := range refs {
		if r.Referrer == "-" {
			t.Errorf("expected '-' referrer to be filtered out")
		}
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 referrers (excluding '-'), got %d", len(refs))
	}
	if refs[0].Referrer != "https://google.com" {
		t.Errorf("top referrer: got %q, want https://google.com", refs[0].Referrer)
	}
}
