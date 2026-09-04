# Contributing to theia

## Prerequisites

Go 1.26.5 or later and [Task](https://taskfile.dev) are required. Verify with `go version` and `task --version`.

## Setup

```bash
git clone https://github.com/Elysium-Labs-EU/theia
cd theia
task setup:setup
```

`task setup:setup` installs the development toolchain (golangci-lint, nilaway, go-crap). Run `task --list` to see all available tasks; always prefer a Task task over raw `go` or tool commands.

## Making Changes

Before touching any function or method, read [STYLE.md](STYLE.md) for the coding conventions that apply to all changes.

Open an issue before starting work on a non-trivial change. This avoids duplicate effort and makes sure the direction fits the project. Small fixes and documentation improvements can go straight to a PR.

Branch from `main` and name the branch after the change: `feat/service-labels`, `fix/restart-backoff`, `test/daemon-lifecycle`.

## Running Tests

```bash
task ci
```

This runs the full test, lint, nilaway, and change-scoped CRAP gate. It must pass before opening a PR. If lint reports violations, `task setup:fix` resolves most of them automatically; run `task ci` again after.

## Commit Format

theia uses [Conventional Commits](https://www.conventionalcommits.org). The prefix determines which section of the changelog the commit appears in.

```
feat: add per-service log retention config
fix: clamp restart backoff to configured max
test: cover daemon shutdown under ctx cancel
refactor: extract systemd unit builder to pure func
docs: document THEIA_VERBOSE env variable
chore: bump golangci-lint to v2.11.0
```

Breaking changes go in the commit footer: `BREAKING CHANGE: <description>`.

## Opening a Pull Request

Fill in the PR template. The summary should explain *why* the change is needed, not just what it does. Link the issue it resolves with `Closes #N`.

All CI checks must be green. A PR that breaks `task ci` will not be reviewed until it is fixed.
