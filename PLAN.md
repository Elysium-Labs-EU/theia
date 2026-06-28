# Plan: `theia stats` CLI Command

## Context

theia is a privacy-first nginx log analytics tool. The daemon ingests logs into SQLite; no CLI exists to read the data back. Users currently need raw `sqlite3` to query their own analytics. This plan adds a `stats` command to complete the CLI story.

---

## Schema (confirmed from migrations)

| Table | Key columns |
|-------|-------------|
| `hourly_stats` | hour, year_day, year, path, host, page_views, unique_visitors, bot_views, is_static |
| `hourly_status_codes` | hour, year_day, year, path, host, status_code, count |
| `hourly_referrers` | hour, year_day, year, path, host, referrer, count |
| `visitor_hashes` | hash, hour_bucket, first_seen |

Time range filter pattern (from `process.go` cleanup functions):
```sql
WHERE (year > ?) OR (year = ? AND year_day >= ?)
```

---

## New Files

### `internal/query/query.go`

Query package — pure read functions. No ingest/write coupling. Defines its own result types.

```go
package query

type Summary struct {
    Pageviews      int
    UniqueVisitors int
    BotViews       int
}

type PathStat struct {
    Path           string
    Host           string
    Pageviews      int
    UniqueVisitors int
}

type StatusStat struct {
    StatusCode int
    Count      int
}

type ReferrerStat struct {
    Referrer string
    Count    int
}

func GetSummary(ctx context.Context, db *sql.DB, since time.Time) (Summary, error)
func GetTopPaths(ctx context.Context, db *sql.DB, since time.Time, limit int) ([]PathStat, error)
func GetStatusCodes(ctx context.Context, db *sql.DB, since time.Time) ([]StatusStat, error)
func GetTopReferrers(ctx context.Context, db *sql.DB, since time.Time, limit int) ([]ReferrerStat, error)
```

Each function uses `db.QueryContext()` + `rows.Next()` + `Scan()` — same pattern as `ingest_test.go`.

Time filter: compute `since.Year()` and `since.YearDay()` then apply the `(year > cutoffYear) OR (year = cutoffYear AND year_day >= cutoffYearDay)` WHERE clause.

### `cmd/stats.go`

Cobra command following exact `daemon.go` pattern:

```
theia stats [flags]

Flags:
  --db-path string   path to the sqlite database (default "./theia.db")
  --days int         number of days to look back (default 7)
  --host string      filter by host (default "", means all hosts)
  --format string    output format: table or json (default "table")
  --top int          number of top paths/referrers to show (default 10)
```

`RunE` body:
1. Parse flags
2. `database.Open(ctx, dbPath)` + `defer database.Close(db)`
3. Compute `since := time.Now().AddDate(0, 0, -days)`
4. Call query functions
5. Render output

### Output rendering (inside `cmd/stats.go`)

**Table format** — uses `text/tabwriter` from stdlib:
```
Summary (last 7 days)
  Pageviews:       12,345
  Unique visitors: 1,023
  Bot views:       342

Top Paths
  PATH                    PAGEVIEWS  VISITORS
  /                       5,000      412
  /about                  2,100      198

Status Codes
  200    11,800
  404    300
  500    12

Top Referrers
  REFERRER              COUNT
  https://example.com   420
  (direct)              8,200
```

**JSON format** — `encoding/json` marshal of a single struct containing all four result sets.

---

## Modified Files

### `cmd/root.go`

Add one line: `rootCmd.AddCommand(newStatsCmd())`

---

## Reused Patterns / Functions

| What | Where |
|------|-------|
| `database.Open()` / `database.Close()` | `database/db.go` |
| `database.RunMigrations()` | `database/migrations.go` — call it so stats works on fresh DB |
| Cobra flag pattern | `cmd/daemon.go` — replicate exactly |
| `db.QueryContext()` + Scan loop | `internal/ingest/ingest_test.go:~L40` |
| Year/year_day time filter | `internal/ingest/process.go:dbCleanUpOldHourlyStats()` |

---

## Verification

```bash
# Build
make build

# Run daemon briefly with a test log, then query
./bin/theia stats --db-path ./theia.db --days 7
./bin/theia stats --db-path ./theia.db --days 30 --format json
./bin/theia stats --db-path ./theia.db --host example.com --top 5

# Tests
go test ./internal/query/... -race -count=2
go test ./cmd/... -race -count=2
```

Add `internal/query/query_test.go` using the same in-memory SQLite test helper pattern from `database/main_test.go` and `internal/ingest/main_test.go`.

---

## Out of Scope

- No `--output-file` flag
- No interactive TUI
- No per-hour breakdown (aggregate only)
- No pagination
