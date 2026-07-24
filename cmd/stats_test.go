package cmd

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Elysium-Labs-EU/theia/database"
	"github.com/spf13/cobra"
)

func setupCmdTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := database.RunMigrations(db, database.MigrationsFS, database.MigrationsPath); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return db, dbPath
}

func insertStat(t *testing.T, db *sql.DB, path, host string, ts time.Time, pageViews, uniqueVisitors, botViews int, isStatic bool) {
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

func newBufCmd() (*cobra.Command, *bytes.Buffer) {
	cmd := &cobra.Command{}
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	return cmd, buf
}

func TestCollectStats(t *testing.T) {
	db, _ := setupCmdTestDB(t)
	defer database.Close(db) //nolint:errcheck // close error in defer is not actionable

	now := time.Now()
	insertStat(t, db, "/", "example.com", now, 5, 3, 1, false)
	insertStat(t, db, "/about", "example.com", now, 3, 2, 0, false)
	insertStat(t, db, "/style.css", "example.com", now, 100, 50, 0, true)

	report, err := collectStats(t.Context(), db, now.AddDate(0, 0, -7), "", 10)
	if err != nil {
		t.Fatalf("collectStats: %v", err)
	}

	if report.Summary.Pageviews != 108 {
		t.Errorf("Pageviews: got %d, want 108", report.Summary.Pageviews)
	}
	if report.Summary.UniqueVisitors != 55 {
		t.Errorf("UniqueVisitors: got %d, want 55", report.Summary.UniqueVisitors)
	}
	if report.Summary.BotViews != 1 {
		t.Errorf("BotViews: got %d, want 1", report.Summary.BotViews)
	}
	if len(report.TopPaths) != 2 {
		t.Errorf("TopPaths: got %d, want 2 (static excluded)", len(report.TopPaths))
	}
}

func TestCollectStats_HostFilter(t *testing.T) {
	db, _ := setupCmdTestDB(t)
	defer database.Close(db) //nolint:errcheck // close error in defer is not actionable

	now := time.Now()
	insertStat(t, db, "/", "example.com", now, 5, 3, 0, false)
	insertStat(t, db, "/", "other.com", now, 10, 7, 0, false)

	report, err := collectStats(t.Context(), db, now.AddDate(0, 0, -7), "example.com", 10)
	if err != nil {
		t.Fatalf("collectStats: %v", err)
	}
	if report.Summary.Pageviews != 5 {
		t.Errorf("Pageviews: got %d, want 5", report.Summary.Pageviews)
	}
}

func TestCollectStats_EmptyDB(t *testing.T) {
	db, _ := setupCmdTestDB(t)
	defer database.Close(db) //nolint:errcheck // close error in defer is not actionable

	report, err := collectStats(t.Context(), db, time.Now().AddDate(0, 0, -7), "", 10)
	if err != nil {
		t.Fatalf("collectStats on empty db: %v", err)
	}
	if report.Summary.Pageviews != 0 || report.Summary.UniqueVisitors != 0 {
		t.Errorf("expected zero summary, got %+v", report.Summary)
	}
}

func TestRenderTable(t *testing.T) {
	cmd, buf := newBufCmd()
	r := &statsReport{}
	r.Summary.Pageviews = 42

	if err := renderTable(cmd, r, 7, ""); err != nil {
		t.Fatalf("renderTable: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"Summary", "Top Paths", "Status Codes", "Top Referrers", "42"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot: %s", want, out)
		}
	}
}

func TestRenderTable_EmptyData(t *testing.T) {
	cmd, buf := newBufCmd()
	r := &statsReport{}

	if err := renderTable(cmd, r, 7, ""); err != nil {
		t.Fatalf("renderTable: %v", err)
	}

	if count := strings.Count(buf.String(), "(no data)"); count != 3 {
		t.Errorf("expected 3 '(no data)' sections, got %d\n%s", count, buf.String())
	}
}

func TestRenderTable_HostInPeriod(t *testing.T) {
	cmd, buf := newBufCmd()
	r := &statsReport{}

	if err := renderTable(cmd, r, 30, "example.com"); err != nil {
		t.Fatalf("renderTable: %v", err)
	}

	if !strings.Contains(buf.String(), "example.com") {
		t.Errorf("expected host in period header\ngot: %s", buf.String())
	}
}

func TestRenderJSON(t *testing.T) {
	cmd, buf := newBufCmd()
	r := &statsReport{}
	r.Summary.Pageviews = 99

	if err := renderJSON(cmd, r); err != nil {
		t.Fatalf("renderJSON: %v", err)
	}

	var got statsReport
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal JSON output: %v\noutput: %s", err, buf.String())
	}
	if got.Summary.Pageviews != 99 {
		t.Errorf("Pageviews: got %d, want 99", got.Summary.Pageviews)
	}
}

func TestStatsCmd_TableFormat(t *testing.T) {
	db, dbPath := setupCmdTestDB(t)
	now := time.Now()
	insertStat(t, db, "/", "example.com", now, 10, 5, 0, false)
	database.Close(db) //nolint:errcheck // close before command reopens the same file

	cmd := newStatsCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--db-path", dbPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\noutput: %s", err, buf.String())
	}

	out := buf.String()
	if !strings.Contains(out, "Pageviews") {
		t.Errorf("expected 'Pageviews' in output\ngot: %s", out)
	}
	if !strings.Contains(out, "10") {
		t.Errorf("expected pageview count '10' in output\ngot: %s", out)
	}
}

func TestStatsCmd_JSONFormat(t *testing.T) {
	db, dbPath := setupCmdTestDB(t)
	now := time.Now()
	insertStat(t, db, "/", "example.com", now, 7, 3, 0, false)
	database.Close(db) //nolint:errcheck // close before command reopens the same file

	cmd := newStatsCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--db-path", dbPath, "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\noutput: %s", err, buf.String())
	}

	var report statsReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal JSON: %v\noutput: %s", err, buf.String())
	}
	if report.Summary.Pageviews != 7 {
		t.Errorf("Pageviews: got %d, want 7", report.Summary.Pageviews)
	}
}

func TestStatsCmd_JSONFormat_EmptyDB(t *testing.T) {
	db, dbPath := setupCmdTestDB(t)
	database.Close(db) //nolint:errcheck // close before command reopens the same file

	cmd := newStatsCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--db-path", dbPath, "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\noutput: %s", err, buf.String())
	}

	out := buf.String()
	if strings.Contains(out, "null") {
		t.Errorf("expected no null fields in JSON output for empty db, got: %s", out)
	}

	var report statsReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal JSON: %v\noutput: %s", err, out)
	}
	if report.TopPaths == nil {
		t.Error("expected top_paths to unmarshal as [], not null")
	}
	if report.StatusCodes == nil {
		t.Error("expected status_codes to unmarshal as [], not null")
	}
	if report.TopReferrers == nil {
		t.Error("expected top_referrers to unmarshal as [], not null")
	}
}

func TestStatsCmd_InvalidDB(t *testing.T) {
	cmd := newStatsCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--db-path", "/nonexistent/path/theia.db"})

	if err := cmd.Execute(); err == nil {
		t.Error("expected error for nonexistent db path, got nil")
	}
}

func TestStatsCmd_EmptyDB(t *testing.T) {
	db, dbPath := setupCmdTestDB(t)
	database.Close(db) //nolint:errcheck // close before command reopens the same file

	cmd := newStatsCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--db-path", dbPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\noutput: %s", err, buf.String())
	}

	if !strings.Contains(buf.String(), "(no data)") {
		t.Errorf("expected '(no data)' for empty db\ngot: %s", buf.String())
	}
}

func TestStatsCmd_Context(t *testing.T) {
	db, dbPath := setupCmdTestDB(t)
	database.Close(db) //nolint:errcheck // close before command reopens the same file

	cmd := newStatsCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--db-path", dbPath, "--days", "30", "--top", "5", "--host", "example.com"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute with all flags: %v\noutput: %s", err, buf.String())
	}
}

func TestCollectStats_Context(t *testing.T) {
	db, _ := setupCmdTestDB(t)
	defer database.Close(db) //nolint:errcheck // close error in defer is not actionable

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := collectStats(ctx, db, time.Now().AddDate(0, 0, -7), "", 10)
	if err == nil {
		t.Error("expected error with canceled context, got nil")
	}
}
