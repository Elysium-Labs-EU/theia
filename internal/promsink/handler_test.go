package promsink_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Elysium-Labs-EU/theia/database"
	"github.com/Elysium-Labs-EU/theia/internal/promsink"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := database.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := database.RunMigrations(db, database.MigrationsFS, database.MigrationsPath); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func insertHourlyStat(t *testing.T, db *sql.DB, path, host string, ts time.Time, pageViews int) {
	t.Helper()
	_, err := db.ExecContext(t.Context(), `
		INSERT INTO hourly_stats (hour, year_day, year, path, host, page_views, is_static)
		VALUES (?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT(hour, year_day, year, path, host) DO UPDATE SET page_views = page_views + ?`,
		ts.Hour(), ts.YearDay(), ts.Year(), path, host, pageViews, pageViews,
	)
	if err != nil {
		t.Fatalf("insert hourly stat: %v", err)
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

func TestHandler_RendersMetrics(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now()
	insertHourlyStat(t, db, "/", "example.com", now, 10)
	insertHourlyStat(t, db, "/", "other.com", now, 4)
	insertStatusCode(t, db, "/", "example.com", now, 200, 42)
	insertReferrer(t, db, "/", "example.com", "https://google.com", now, 7)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	promsink.Handler(db, 20)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type: got %q, want text/plain prefix", ct)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`theia_pageviews_total{host="example.com",path="/"} 10`,
		`theia_pageviews_total{host="other.com",path="/"} 4`,
		`theia_status_codes_total{status_code="200"} 42`,
		`theia_referrers_total{referrer="https://google.com"} 7`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %q, got:\n%s", want, body)
		}
	}
}

func TestHandler_EmptyDB(t *testing.T) {
	db := setupTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	promsink.Handler(db, 20)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "theia_pageviews_total") {
		t.Errorf("expected metric families in output even with no data, got:\n%s", rec.Body.String())
	}
}

func TestHandler_QueryError(t *testing.T) {
	db := setupTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	promsink.Handler(db, 20)(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want 500, body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_TopBoundsCardinality(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now()
	insertHourlyStat(t, db, "/a", "example.com", now, 3)
	insertHourlyStat(t, db, "/b", "example.com", now, 2)
	insertHourlyStat(t, db, "/c", "example.com", now, 1)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	promsink.Handler(db, 1)(rec, req)

	body := rec.Body.String()
	if strings.Count(body, "theia_pageviews_total{") != 1 {
		t.Errorf("expected exactly 1 pageview series with top=1, got:\n%s", body)
	}
	if !strings.Contains(body, `path="/a"`) {
		t.Errorf("expected the highest-count path (/a) to survive the top=1 cap, got:\n%s", body)
	}
}
