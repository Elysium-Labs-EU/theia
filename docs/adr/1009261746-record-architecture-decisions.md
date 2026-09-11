# 1009261746. Record architecture decisions

Date: 2026-09-10
Status: Accepted

## Context

- Design decisions and their tradeoffs live only in commit messages, issues,
  and chat history today. Issues close and get buried, chat isn't durable
  project state, and a commit message is easy to miss skimming `git log`.

## Decision

- Record significant architecture/infra decisions as ADRs under `docs/adr/`,
  one file per decision, id `docs/adr/DDMMYYHHMM-slug.md`, immutable once
  accepted. A reversal is a new ADR that supersedes the old one.
- No shared index file — the directory listing plus the `docs:adr-find`
  task (or a plain `grep`) is the index.
- `Status:` is always an inline line, never a heading.
- Sections: Context, Decision, Rejected (when relevant), Consequences.
  Bullets, not prose. No issue numbers in the body.

## Consequences

- Adds one file per real decision, maintained by hand, no CI enforcement.
- `task docs:adr-find -- <concept>` (or a plain `grep`) is the only way to
  check status; nothing else needs to stay in sync.
