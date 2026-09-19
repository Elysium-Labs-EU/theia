# 1609261032. Referrer filtering at query time

Date: 2026-09-16
Status: Accepted

## Context

- `theia stats`'s Top Referrers report surfaces the raw `Referer` header a
  client sent — a value the server never verifies. A bot can set it to
  anything (e.g. `binance.com`) hoping to show up in someone's analytics.
  There was no way to drop a known-spam or otherwise unwanted referrer from
  the report, unlike host and path (see the host/path filtering ADR).
- `query.Filters` already carries `Host`/`ExcludeHosts`/`ExcludePaths` and is
  threaded through the four `query` functions `theia stats` calls.

## Decision

- Add `ExcludeReferrers []string` to the existing `query.Filters` struct and
  a matching `--exclude-referrer` flag on `theia stats` (repeatable, same as
  `--exclude-host`/`--exclude-path`).
- Query-time only — no ingest-time equivalent. Unlike host/path, a referrer
  isn't a routing/storage decision; it's the data point itself, and
  `hourly_referrers` is the only table that has a `referrer` column at all.
  Keeping it query-time also means excluding a referrer later doesn't lose
  data the way an ingest-time filter would.
- Matching is exact string equality, not prefix or glob/regex — spam
  referrer values are typically exact domains (`binance.com`), not paths
  with meaningful prefixes, so prefix matching doesn't fit and glob/regex
  was already rejected in the host/path filtering ADR for the same
  complexity/injection-surface reasons.
- `filterClause` gained a second gate, `withReferrer`, alongside the
  existing `withPath`: only `GetTopReferrers` passes `withReferrer: true`,
  since `hourly_stats`, `hourly_status_codes`, and `visitor_days` have no
  `referrer` column.
- `query.Filters` is now passed by pointer (`*Filters`) throughout
  `internal/query` and by `cmd/stats.go`. Adding a fourth `[]string` field
  pushed the struct's copy cost over golangci-lint's `gocritic: hugeParam`
  threshold; the callers never mutate it, so this is the same
  avoid-a-copy-not-signal-nil exception already used for `ingest.PageView`
  in the host/path filtering change.

## Rejected

- **Ingest-time referrer filtering.** Storage isn't the problem (referrer
  spam doesn't meaningfully grow the database the way unwanted host/path
  traffic does), and excluding a referrer permanently would mean losing the
  ability to ever report on it again if the exclusion was a mistake.
- **Prefix or glob/regex matching.** Referrer spam values are exact domains,
  not hierarchical paths; exact match covers the actual need without adding
  a matching engine.

## Consequences

- `query.GetSummary`, `getUniqueVisitors`, `GetTopPaths`, `GetStatusCodes`,
  and `GetTopReferrers` all now take `*Filters` instead of `Filters` —
  ripples through every caller and test in `internal/query` and
  `cmd/stats.go`, but is a mechanical, behavior-preserving change.
- Operators can drop spoofed/spam referrer values from `theia stats`
  reports without touching the database.
