# 1009261747. Host and path filtering at ingest time and query time

Date: 2026-09-10
Status: Accepted

## Context

- theia's only existing filtering knob is `--host` on `stats`/`serve`/
  `serve-metrics`: a single positive include applied at query time.
- There is no way to exclude a host, no path filtering anywhere, and no
  ingest-time filtering at all — the daemon stores every line from the
  nginx access log unconditionally.
- A single nginx access log commonly carries traffic for several vhosts on
  one host (e.g. an internal media server's API polling alongside the
  actual site being measured). That traffic pollutes every report today,
  with no way to keep it out of the database in the first place.
- `visitor_days` (backing the unique-visitors count) is keyed on
  `(host, day)` only — it has no `path` column. Any design that adds path
  filtering has to account for unique-visitor counts being inherently
  host-day-grained, not per-path.

## Decision

- Add filtering at two independent scopes, both kept:
  - **Ingest time** (`theia daemon`): `--include-host`, `--exclude-host`,
    `--exclude-path` flags. A line failing the filter is parsed but never
    written to the database — permanent, keeps the db small, but doesn't
    retroactively clean up anything already stored.
  - **Query time** (`theia stats`): `--exclude-host`, `--exclude-path`
    flags, additive to the existing single-value `--host` include.
    Existing data stays in the db; reports can still drop noisy hosts/
    paths without re-ingesting.
- Host matching is by normalized (lowercased) exact string; path matching
  is by prefix, mirroring the existing `isStaticAsset` prefix-check style
  already used in the parser rather than introducing full glob/regex
  matching.
- Ingest-time filtering is a pure predicate (`PageView -> bool`) evaluated
  after parsing, before the row is pushed onto the ingest channel — kept
  separate from parsing itself so parse failures and filter decisions stay
  independently testable.
- Query-time filtering only touches the four `query` functions `theia
  stats` actually calls (`GetSummary`, `GetTopPaths`, `GetStatusCodes`,
  `GetTopReferrers`, plus the internal `getUniqueVisitors`) — not the
  `*Range`/`GetSeries` functions that back `serve`'s HTTP API, which stay
  out of scope for this change.
- Query-time path exclusion applies only where a `path` column exists
  (`hourly_stats`, `hourly_status_codes`, `hourly_referrers`); unique
  visitor counts, sourced from `visitor_days`, only ever respect host
  filtering — this is accepted as a schema property, not treated as a bug
  to work around.

## Rejected

- **Query-time only.** Would satisfy the "hide it in reports" need but
  leaves the database growing unbounded with data the operator has already
  said they don't want tracked at all.
- **Ingest-time only.** Permanent and irreversible — an operator who wants
  to later report on data they excluded (or excluded a host by mistake)
  has no way back without re-ingesting from raw logs, which theia doesn't
  support (no replay/offset tracking).
- **Glob/regex path matching.** More flexible, but adds a matching engine
  and its own injection/perf surface for a need (drop a known noisy path
  prefix like `/rest/ping`) that plain prefix matching already covers.
- **Extending query-time filtering to `serve`'s HTTP API in the same
  change.** Keeps this change scoped to what was actually asked for (the
  CLI); the API's `*Range`/`GetSeries` path can pick up the same `Filters`
  shape later without redesign.

## Consequences

- `query.GetSummary`, `getUniqueVisitors`, `GetTopPaths`, `GetStatusCodes`,
  and `GetTopReferrers` take a `Filters` struct instead of a bare `host
  string` — their only caller, `cmd/stats.go`, updates alongside.
- `internal/ingest.Run` takes an additional filter parameter, threaded
  through to `tailLog`; `cmd/daemon.go` builds it from the new flags.
- The HTTP API (`serve`) is unaffected until a follow-up change threads the
  same `Filters` shape through `*Range`/`GetSeries`.
- Operators with a shared access log across vhosts get a real way to keep
  unrelated traffic out of both storage and reports.
