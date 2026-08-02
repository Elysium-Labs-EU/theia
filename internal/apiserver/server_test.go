package apiserver_test

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Elysium-Labs-EU/theia/database"
	"github.com/Elysium-Labs-EU/theia/internal/apiserver"
)

const testToken = "test-token-123"

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

type statSeed struct {
	PageViews      int
	UniqueVisitors int
	BotViews       int
}

func insertHourlyStat(t *testing.T, db *sql.DB, path, host string, ts time.Time, s statSeed) {
	t.Helper()
	_, err := db.ExecContext(t.Context(), `
		INSERT INTO hourly_stats (hour, year_day, year, path, host, page_views, is_static, bot_views)
		VALUES (?, ?, ?, ?, ?, ?, 0, ?)
		ON CONFLICT(hour, year_day, year, path, host) DO UPDATE SET
			page_views = page_views + ?,
			bot_views = bot_views + ?`,
		ts.Hour(), ts.YearDay(), ts.Year(), path, host,
		s.PageViews, s.BotViews,
		s.PageViews, s.BotViews,
	)
	if err != nil {
		t.Fatalf("insert hourly stat: %v", err)
	}

	for i := range s.UniqueVisitors {
		hash := path + host + string(rune('a'+i))
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

func doRequest(t *testing.T, handler http.Handler, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestStats_Unauthorized(t *testing.T) {
	db := setupTestDB(t)
	srv := apiserver.NewServer(db, apiserver.Config{Token: testToken})

	for _, tt := range []struct {
		name  string
		token string
	}{
		{"missing token", ""},
		{"wrong token", "wrong-token"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := doRequest(t, srv.Handler, "/api/v1/stats", tt.token)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestStats_JSON(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now()
	insertHourlyStat(t, db, "/", "example.com", now, statSeed{PageViews: 5, UniqueVisitors: 3, BotViews: 1})

	srv := apiserver.NewServer(db, apiserver.Config{Token: testToken})
	rec := doRequest(t, srv.Handler, "/api/v1/stats?host=example.com", testToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}

	var got struct {
		Host    string `json:"host"`
		GroupBy string `json:"group_by"`
		Series  []struct {
			Date           string `json:"date"`
			PageViews      int    `json:"page_views"`
			UniqueVisitors int    `json:"unique_visitors"`
			BotViews       int    `json:"bot_views"`
		} `json:"series"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %s", err, rec.Body.String())
	}
	if got.Host != "example.com" {
		t.Errorf("host: got %q, want example.com", got.Host)
	}
	if got.GroupBy != "day" {
		t.Errorf("group_by: got %q, want day", got.GroupBy)
	}
	if len(got.Series) != 1 {
		t.Fatalf("series: got %d points, want 1: %+v", len(got.Series), got.Series)
	}
	if got.Series[0].PageViews != 5 || got.Series[0].UniqueVisitors != 3 || got.Series[0].BotViews != 1 {
		t.Errorf("series point: got %+v, want pv=5 uv=3 bv=1", got.Series[0])
	}
}

func TestStats_HostFilter(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now()
	insertHourlyStat(t, db, "/", "example.com", now, statSeed{PageViews: 5, UniqueVisitors: 3, BotViews: 0})
	insertHourlyStat(t, db, "/", "other.com", now, statSeed{PageViews: 10, UniqueVisitors: 7, BotViews: 0})

	srv := apiserver.NewServer(db, apiserver.Config{Token: testToken})
	rec := doRequest(t, srv.Handler, "/api/v1/stats?host=other.com", testToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200, body: %s", rec.Code, rec.Body.String())
	}

	var got struct {
		Series []struct {
			PageViews int `json:"page_views"`
		} `json:"series"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Series) != 1 || got.Series[0].PageViews != 10 {
		t.Fatalf("expected other.com's 10 pageviews only, got %+v", got.Series)
	}
}

func TestStats_CSV(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now()
	insertHourlyStat(t, db, "/", "example.com", now, statSeed{PageViews: 5, UniqueVisitors: 3, BotViews: 1})

	srv := apiserver.NewServer(db, apiserver.Config{Token: testToken})
	rec := doRequest(t, srv.Handler, "/api/v1/stats?format=csv", testToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type: got %q, want text/csv prefix", ct)
	}

	rows, err := csv.NewReader(rec.Body).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected header + 1 data row, got %d rows: %v", len(rows), rows)
	}
	if want := []string{"date", "page_views", "unique_visitors", "bot_views"}; !equalSlices(rows[0], want) {
		t.Errorf("header: got %v, want %v", rows[0], want)
	}
	if rows[1][1] != "5" || rows[1][2] != "3" || rows[1][3] != "1" {
		t.Errorf("data row: got %v, want pv=5 uv=3 bv=1", rows[1])
	}
}

func TestStats_BadParams(t *testing.T) {
	db := setupTestDB(t)
	srv := apiserver.NewServer(db, apiserver.Config{Token: testToken})

	for _, tt := range []struct {
		name string
		path string
	}{
		{"bad from date", "/api/v1/stats?from=not-a-date"},
		{"bad to date", "/api/v1/stats?to=not-a-date"},
		{"from after to", "/api/v1/stats?from=2026-07-14&to=2026-06-01"},
		{"invalid group_by", "/api/v1/stats?group_by=week"},
		{"invalid format", "/api/v1/stats?format=xml"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := doRequest(t, srv.Handler, tt.path, testToken)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status: got %d, want 400, body: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestPaths_JSON(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now()
	insertHourlyStat(t, db, "/", "example.com", now, statSeed{PageViews: 10, UniqueVisitors: 5, BotViews: 0})
	insertHourlyStat(t, db, "/about", "example.com", now, statSeed{PageViews: 3, UniqueVisitors: 1, BotViews: 0})

	srv := apiserver.NewServer(db, apiserver.Config{Token: testToken})
	rec := doRequest(t, srv.Handler, "/api/v1/stats/paths?top=1", testToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200, body: %s", rec.Code, rec.Body.String())
	}

	var got struct {
		Paths []struct {
			Path      string `json:"path"`
			PageViews int    `json:"page_views"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Paths) != 1 || got.Paths[0].Path != "/" || got.Paths[0].PageViews != 10 {
		t.Fatalf("expected top path '/' with 10 views, got %+v", got.Paths)
	}
}

func TestReferrers_JSON(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now()
	insertReferrer(t, db, "/", "example.com", "https://google.com", now, 7)

	srv := apiserver.NewServer(db, apiserver.Config{Token: testToken})
	rec := doRequest(t, srv.Handler, "/api/v1/stats/referrers", testToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200, body: %s", rec.Code, rec.Body.String())
	}

	var got struct {
		Referrers []struct {
			Referrer string `json:"referrer"`
			Count    int    `json:"count"`
		} `json:"referrers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Referrers) != 1 || got.Referrers[0].Referrer != "https://google.com" || got.Referrers[0].Count != 7 {
		t.Fatalf("expected google.com referrer with count 7, got %+v", got.Referrers)
	}
}

func TestStatusCodes_JSON(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now()
	insertStatusCode(t, db, "/", "example.com", now, 200, 42)

	srv := apiserver.NewServer(db, apiserver.Config{Token: testToken})
	rec := doRequest(t, srv.Handler, "/api/v1/stats/status-codes", testToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200, body: %s", rec.Code, rec.Body.String())
	}

	var got struct {
		StatusCodes []struct {
			StatusCode int `json:"status_code"`
			Count      int `json:"count"`
		} `json:"status_codes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.StatusCodes) != 1 || got.StatusCodes[0].StatusCode != 200 || got.StatusCodes[0].Count != 42 {
		t.Fatalf("expected status 200 with count 42, got %+v", got.StatusCodes)
	}
}

func TestBreakdown_BadTop(t *testing.T) {
	db := setupTestDB(t)
	srv := apiserver.NewServer(db, apiserver.Config{Token: testToken})

	for _, path := range []string{
		"/api/v1/stats/paths?top=0",
		"/api/v1/stats/paths?top=abc",
		"/api/v1/stats/referrers?top=-1",
	} {
		rec := doRequest(t, srv.Handler, path, testToken)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status got %d, want 400", path, rec.Code)
		}
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
