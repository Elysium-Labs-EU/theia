package ingest

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Elysium-Labs-EU/theia/database"
)

// expectedHourlyStat describes the expected shape of a single hourly_stats row
// in the ingest-pipeline scenario tests below.
type expectedHourlyStat struct {
	Pageviews int
	BotViews  int
	IsStatic  bool
}

// cleanupCounts describes the expected row counts across the hourly/visitor
// tables after a periodic cleanup pass.
type cleanupCounts struct {
	hourlyStats       int
	hourlyStatusCodes int
	hourlyReferrers   int
	visitorDays       int
}

// runIngestScenario spins up a fresh test database, writes logLines to a temp
// access log, and runs them through the tail -> pageview-processing pipeline.
// It returns the resulting database for assertions.
func runIngestScenario(t *testing.T, logLines []string) *sql.DB {
	t.Helper()
	db, tempDir := setupTestDB(t)
	t.Cleanup(func() {
		_ = database.Close(db)
	})

	logPath := filepath.Join(tempDir, "access.log")
	createTestLogFile(t, logPath, logLines)

	pageViews := make(chan PageView, 100)

	var wg sync.WaitGroup
	wg.Add(1)
	go processPageviewsWithWaitGroup(t.Context(), db, pageViews, &wg)

	tailArgs := []string{"-n", "+1", logPath}
	if err := tailLog(t.Context(), tailArgs, pageViews); err != nil {
		t.Fatalf("tailLog: %v", err)
	}
	close(pageViews)
	wg.Wait()

	return db
}

// wantScenarioEntries is the number of test log lines every ingest scenario
// test below writes, and therefore the row count expected in each table
// before periodic cleanup runs.
const wantScenarioEntries = 3

func assertVisitorDayCount(t *testing.T, db *sql.DB) []VisitorDay {
	t.Helper()
	visitorDays := getVisitorDays(t, db)
	if len(visitorDays) != wantScenarioEntries {
		t.Fatalf("Expected %d entries in visitor day table, got %d instead", wantScenarioEntries, len(visitorDays))
	}
	return visitorDays
}

func assertAllVisitorDaysOnYearDay(t *testing.T, visitorDays []VisitorDay, year, yearDay int) {
	t.Helper()
	for _, visitorDay := range visitorDays {
		if visitorDay.Year != year || visitorDay.YearDay != yearDay {
			t.Errorf("Expected all the entries to be recorded on year_day %d of %d", yearDay, year)
			return
		}
	}
}

func assertHourlyStatEntry(t *testing.T, i int, got *HourlyStats, exp expectedHourlyStat) {
	t.Helper()
	if got.Pageviews != exp.Pageviews {
		t.Errorf("Entry %d: expected %d page views, got %d instead", i, exp.Pageviews, got.Pageviews)
	}
	if got.BotViews != exp.BotViews {
		t.Errorf("Entry %d: expected %d bot views, got %d instead", i, exp.BotViews, got.BotViews)
	}
	if got.IsStatic != exp.IsStatic {
		t.Errorf("Entry %d: expected is_static=%v, got %v instead", i, exp.IsStatic, got.IsStatic)
	}
}

func assertHourlyStats(t *testing.T, db *sql.DB, want []expectedHourlyStat) {
	t.Helper()
	hourlyStats := getHourlyStats(t, db)
	if len(hourlyStats) != len(want) {
		t.Fatalf("Expected %d entries in hourly stats table, got %d instead", len(want), len(hourlyStats))
	}
	for i, exp := range want {
		assertHourlyStatEntry(t, i, &hourlyStats[i], exp)
	}
}

func assertAllStatusCodesAre200(t *testing.T, db *sql.DB) {
	t.Helper()
	const wantCode = 200
	hourlyStatusCodes := getHourlyStatusCodes(t, db)
	if len(hourlyStatusCodes) != wantScenarioEntries {
		t.Errorf("Expected %d entries in hourly status codes table, got %d instead", wantScenarioEntries, len(hourlyStatusCodes))
	}
	for _, hourlyStatusCode := range hourlyStatusCodes {
		if hourlyStatusCode.StatusCode != wantCode {
			t.Errorf("Expected all the entries to have %d status code", wantCode)
			return
		}
	}
}

func assertAllReferrerCountsAreOne(t *testing.T, db *sql.DB) {
	t.Helper()
	const wantCount = 1
	hourlyReferrers := getHourlyReferrers(t, db)
	if len(hourlyReferrers) != wantScenarioEntries {
		t.Errorf("Expected %d entries in hourly referrers table, got %d instead", wantScenarioEntries, len(hourlyReferrers))
	}
	for _, hourlyReferrer := range hourlyReferrers {
		if hourlyReferrer.Count != wantCount {
			t.Errorf("Expected all the entries to have %d as count", wantCount)
			return
		}
	}
}

func assertTableCount(t *testing.T, db *sql.DB, table, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}
	if got != want {
		t.Errorf("Expected %d %s record remaining, got %d", want, table, got)
	}
}

func runPeriodicCleanupAndAssertCounts(t *testing.T, db *sql.DB, want cleanupCounts) {
	t.Helper()
	ticker := time.NewTicker(1 * time.Second)
	shutdown := make(chan os.Signal, 1)

	var wg sync.WaitGroup
	wg.Add(1)
	go runPeriodicCleanupsWithWaitGroup(t.Context(), db, ticker, shutdown, &wg)

	shutdown <- syscall.SIGTERM
	close(shutdown)
	wg.Wait()

	assertTableCount(t, db, "hourly_stats", "SELECT COUNT(*) FROM hourly_stats", want.hourlyStats)
	assertTableCount(t, db, "hourly_status_codes", "SELECT COUNT(*) FROM hourly_status_codes", want.hourlyStatusCodes)
	assertTableCount(t, db, "hourly_referrers", "SELECT COUNT(*) FROM hourly_referrers", want.hourlyReferrers)
	assertTableCount(t, db, "visitor_days", "SELECT COUNT(*) FROM visitor_days", want.visitorDays)
}

func TestRunSameDay20DaysAgo(t *testing.T) {
	testLogLines := []string{
		fmt.Sprintf(`192.168.1.1 - - [%s] "GET /index.html HTTP/1.1" 200 1234 "https://google.com" "Mozilla/5.0" "example.com"`, time.Now().AddDate(0, 0, -20).Format("02/Jan/2006:15:04:05 -0700")),
		fmt.Sprintf(`10.0.0.5 - - [%s] "GET /api/data HTTP/1.1" 200 5678 "-" "curl/7.68.0" "example.com"`, time.Now().Add(15*time.Second).AddDate(0, 0, -20).Format("02/Jan/2006:15:04:05 -0700")),
		fmt.Sprintf(`192.168.1.100 - - [%s] "GET /style.css HTTP/1.1" 200 900 "https://example.com" "Mozilla/5.0" "example.com"`, time.Now().Add(15*time.Second).AddDate(0, 0, -20).Format("02/Jan/2006:15:04:05 -0700")),
	}
	db := runIngestScenario(t, testLogLines)

	assertVisitorDayCount(t, db)
	assertHourlyStats(t, db, []expectedHourlyStat{
		{Pageviews: 1, BotViews: 0, IsStatic: false},
		{Pageviews: 0, BotViews: 1, IsStatic: false},
		{Pageviews: 1, BotViews: 0, IsStatic: true},
	})
	assertAllStatusCodesAre200(t, db)
	assertAllReferrerCountsAreOne(t, db)
	runPeriodicCleanupAndAssertCounts(t, db, cleanupCounts{hourlyStats: 3, hourlyStatusCodes: 3, hourlyReferrers: 3, visitorDays: 3})
}

func TestRunSameDayInThePast(t *testing.T) {
	testLogLines := []string{
		`192.168.1.1 - - [24/Dec/2024:10:30:45 +0000] "GET /index.html HTTP/1.1" 200 1234 "https://google.com" "Mozilla/5.0" "example.com"`,
		`10.0.0.5 - - [24/Dec/2024:10:31:00 +0000] "GET /api/data HTTP/1.1" 200 5678 "-" "curl/7.68.0" "example.com"`,
		`192.168.1.100 - - [24/Dec/2024:10:31:15 +0000] "GET /style.css HTTP/1.1" 200 900 "https://example.com" "Mozilla/5.0" "example.com"`,
	}
	db := runIngestScenario(t, testLogLines)

	visitorDays := assertVisitorDayCount(t, db)
	assertAllVisitorDaysOnYearDay(t, visitorDays, 2024, 359)
	assertHourlyStats(t, db, []expectedHourlyStat{
		{Pageviews: 1, BotViews: 0, IsStatic: false},
		{Pageviews: 0, BotViews: 1, IsStatic: false},
		{Pageviews: 1, BotViews: 0, IsStatic: true},
	})
	assertAllStatusCodesAre200(t, db)
	assertAllReferrerCountsAreOne(t, db)
	runPeriodicCleanupAndAssertCounts(t, db, cleanupCounts{hourlyStats: 0, hourlyStatusCodes: 0, hourlyReferrers: 0, visitorDays: 0})
}

func TestRunDifferentDaysInNearPast(t *testing.T) {
	testLogLines := []string{
		fmt.Sprintf(`192.168.1.1 - - [%s] "GET /index.html HTTP/1.1" 200 1234 "https://google.com" "Mozilla/5.0" "example.com"`, time.Now().AddDate(0, 0, 0).Format("02/Jan/2006:15:04:05 -0700")),
		fmt.Sprintf(`10.0.0.5 - - [%s] "GET /api/data HTTP/1.1" 200 5678 "-" "curl/7.68.0" "example.com"`, time.Now().Add(15*time.Second).AddDate(0, 0, -1).Format("02/Jan/2006:15:04:05 -0700")),
		fmt.Sprintf(`192.168.1.100 - - [%s] "GET /style.css HTTP/1.1" 200 900 "https://example.com" "Mozilla/5.0" "example.com"`, time.Now().Add(15*time.Second).AddDate(0, 0, -2).Format("02/Jan/2006:15:04:05 -0700")),
	}
	db := runIngestScenario(t, testLogLines)

	assertVisitorDayCount(t, db)
	assertHourlyStats(t, db, []expectedHourlyStat{
		{Pageviews: 1, BotViews: 0, IsStatic: false},
		{Pageviews: 0, BotViews: 1, IsStatic: false},
		{Pageviews: 1, BotViews: 0, IsStatic: true},
	})
	assertAllStatusCodesAre200(t, db)
	assertAllReferrerCountsAreOne(t, db)
	runPeriodicCleanupAndAssertCounts(t, db, cleanupCounts{hourlyStats: 3, hourlyStatusCodes: 3, hourlyReferrers: 3, visitorDays: 3})
}

func TestRunDifferentDaysInDistantPast(t *testing.T) {
	testLogLines := []string{
		fmt.Sprintf(`192.168.1.1 - - [%s] "GET /index.html HTTP/1.1" 200 1234 "https://google.com" "Mozilla/5.0" "example.com"`, time.Now().AddDate(0, 0, -20).Format("02/Jan/2006:15:04:05 -0700")),
		fmt.Sprintf(`10.0.0.5 - - [%s] "GET /api/data HTTP/1.1" 200 5678 "-" "curl/7.68.0" "example.com"`, time.Now().Add(15*time.Second).AddDate(0, 0, -59).Format("02/Jan/2006:15:04:05 -0700")),
		fmt.Sprintf(`192.168.1.100 - - [%s] "GET /style.css HTTP/1.1" 200 900 "https://example.com" "Mozilla/5.0" "example.com"`, time.Now().Add(15*time.Second).AddDate(0, 0, -61).Format("02/Jan/2006:15:04:05 -0700")),
	}
	db := runIngestScenario(t, testLogLines)

	assertVisitorDayCount(t, db)
	assertHourlyStats(t, db, []expectedHourlyStat{
		{Pageviews: 1, BotViews: 0, IsStatic: false},
		{Pageviews: 0, BotViews: 1, IsStatic: false},
		{Pageviews: 1, BotViews: 0, IsStatic: true},
	})
	assertAllStatusCodesAre200(t, db)
	assertAllReferrerCountsAreOne(t, db)
	runPeriodicCleanupAndAssertCounts(t, db, cleanupCounts{hourlyStats: 2, hourlyStatusCodes: 2, hourlyReferrers: 2, visitorDays: 2})
}

func newTestDB(ctx context.Context, dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, fmt.Errorf("could not create test db directory: %w", err)
	}
	return database.Open(ctx, dbPath)
}

func setupTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	db, err := newTestDB(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("Unable to create test database: %v", err)
	}

	if migrationsErr := database.RunMigrations(db, database.MigrationsFS, database.MigrationsPath); migrationsErr != nil {
		t.Fatalf("failed to run migrations: %v", migrationsErr)
	}

	_, dirty, err := database.GetCurrentVersion(db, database.MigrationsFS, database.MigrationsPath)
	if err != nil {
		fmt.Printf("Warning: Could not get schema version: %v\n", err)
	} else if dirty {
		t.Fatal("Database is in a dirty state. Manual intervention required.")
	}

	return db, tempDir
}

func processPageviewsWithWaitGroup(ctx context.Context, db *sql.DB, pageViews <-chan PageView, wg *sync.WaitGroup) {
	defer wg.Done()
	processPageviews(ctx, db, pageViews)
}

func runPeriodicCleanupsWithWaitGroup(ctx context.Context, db *sql.DB, ticker *time.Ticker, shutdown <-chan os.Signal, wg *sync.WaitGroup) {
	defer wg.Done()
	runPeriodicCleanup(ctx, db, ticker, shutdown)
}

func createTestLogFile(t *testing.T, logPath string, logLines []string) {
	t.Helper()

	dir := filepath.Dir(logPath)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		t.Fatalf("Failed to create log directory: %v", err)
	}

	file, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("Failed to create log file: %v", err)
	}
	defer file.Close() //nolint:errcheck // close error in defer is not actionable

	for _, line := range logLines {
		_, err := file.WriteString(line + "\n")
		if err != nil {
			t.Fatalf("Failed to write log line: %v", err)
		}
	}
}

func getVisitorDays(t *testing.T, db *sql.DB) []VisitorDay {
	t.Helper()

	visitorDaysQuery := `SELECT * FROM visitor_days`
	visitorRows, err := db.QueryContext(t.Context(), visitorDaysQuery)
	if err != nil {
		t.Fatalf("could not query the database for vistor hashes: %v", err)
	}
	defer visitorRows.Close() //nolint:errcheck // close error in defer is not actionable
	var visitorDays []VisitorDay
	for visitorRows.Next() {
		var visitorDay VisitorDay
		err := visitorRows.Scan(
			&visitorDay.Hash,
			&visitorDay.Host,
			&visitorDay.Year,
			&visitorDay.YearDay,
			&visitorDay.FirstSeen,
		)
		if err != nil {
			t.Fatalf("unable to parse database visitor day output, %v", err)
		}
		visitorDays = append(visitorDays, visitorDay)
	}
	if err := visitorRows.Err(); err != nil {
		t.Fatalf("visitorRows iteration error: %v", err)
	}
	return visitorDays
}

func getHourlyStats(t *testing.T, db *sql.DB) []HourlyStats {
	t.Helper()

	hourlyStatsQuery := `SELECT * FROM hourly_stats`
	hourlyStatsRows, err := db.QueryContext(t.Context(), hourlyStatsQuery)
	if err != nil {
		t.Fatalf("could not query the database for hourly stats: %v", err)
	}
	defer hourlyStatsRows.Close() //nolint:errcheck // close error in defer is not actionable
	var hourlyStats []HourlyStats
	for hourlyStatsRows.Next() {
		var hourlyStat HourlyStats
		err := hourlyStatsRows.Scan(
			&hourlyStat.Hour,
			&hourlyStat.YearDay,
			&hourlyStat.Year,
			&hourlyStat.Path,
			&hourlyStat.Host,
			&hourlyStat.Pageviews,
			&hourlyStat.IsStatic,
			&hourlyStat.BotViews,
		)
		if err != nil {
			t.Fatalf("unable to parse database hourly stat output, %v", err)
		}
		hourlyStats = append(hourlyStats, hourlyStat)
	}
	if err := hourlyStatsRows.Err(); err != nil {
		t.Fatalf("hourlyStatsRows iteration error: %v", err)
	}
	return hourlyStats
}

func getHourlyStatusCodes(t *testing.T, db *sql.DB) []HourlyStatusCodes {
	t.Helper()

	hourlyStatusCodesQuery := `SELECT * FROM hourly_status_codes`
	hourlyStatusCodesRows, err := db.QueryContext(t.Context(), hourlyStatusCodesQuery)
	if hourlyStatusCodesRows.Err() != nil {
		t.Fatalf("could not query the database for hourly status codes: %v", err)
	}
	if err != nil {
		t.Fatalf("could not query the database for hourly status codes: %v", err)
	}
	defer hourlyStatusCodesRows.Close() //nolint:errcheck // close error in defer is not actionable
	var hourlyStatusCodes []HourlyStatusCodes
	for hourlyStatusCodesRows.Next() {
		var hourlyStatusCode HourlyStatusCodes
		err := hourlyStatusCodesRows.Scan(
			&hourlyStatusCode.Hour,
			&hourlyStatusCode.YearDay,
			&hourlyStatusCode.Year,
			&hourlyStatusCode.Path,
			&hourlyStatusCode.Host,
			&hourlyStatusCode.StatusCode,
			&hourlyStatusCode.Count,
		)
		if err != nil {
			t.Fatalf("unable to parse database hourly status code output, %v", err)
		}
		hourlyStatusCodes = append(hourlyStatusCodes, hourlyStatusCode)
	}
	return hourlyStatusCodes
}

func getHourlyReferrers(t *testing.T, db *sql.DB) []HourlyReferrers {
	t.Helper()

	hourlyReferrersQuery := `SELECT * FROM hourly_referrers`
	hourlyReferrersRows, err := db.QueryContext(t.Context(), hourlyReferrersQuery)
	if err != nil {
		t.Fatalf("could not query the database for hourly referrers: %v", err)
	}

	defer hourlyReferrersRows.Close() //nolint:errcheck // close error in defer is not actionable
	var hourlyReferrers []HourlyReferrers
	for hourlyReferrersRows.Next() {
		var hourlyReferrer HourlyReferrers
		err := hourlyReferrersRows.Scan(
			&hourlyReferrer.Hour,
			&hourlyReferrer.YearDay,
			&hourlyReferrer.Year,
			&hourlyReferrer.Path,
			&hourlyReferrer.Host,
			&hourlyReferrer.Referrer,
			&hourlyReferrer.Count,
		)
		rowsErr := hourlyReferrersRows.Err()
		if rowsErr != nil {
			t.Fatalf("unable to parse database hourly referrers output, %v", err)
		}
		if err != nil {
			t.Fatalf("unable to parse database hourly referrers output, %v", err)
		}
		hourlyReferrers = append(hourlyReferrers, hourlyReferrer)
	}
	return hourlyReferrers
}
