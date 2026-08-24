#!/usr/bin/env bash
# Fails if golangci-lint's version stops having exactly one source of truth.
#
# The version lives in .golangci-lint-version and is read by the Makefile's
# lint/fix targets, the pre-commit hook, and any CI workflow that lints.
# Nothing about that arrangement is self-enforcing: a hardcoded version in a
# workflow, or a bare `golangci-lint` invocation resolving from PATH,
# reintroduces the split quietly and only shows up when two of them disagree
# about a specific line.
#
# That disagreement is not hypothetical. In this repo three versions were
# in play at once (Makefile v2.11.0, CI workflow v2.11.4, lefthook.yml
# hardcoded to v2.11.0 separately) before this guard existed.
set -euo pipefail

readonly VERSION_FILE=".golangci-lint-version"
failures=0

fail() {
    echo "check-golangci-pin: $1" >&2
    failures=$((failures + 1))
}

if [ ! -f "$VERSION_FILE" ]; then
    fail "${VERSION_FILE} is missing; it is the single source of the linter version"
    echo "check-golangci-pin: ${failures} problem(s) found" >&2
    exit 1
fi

version="$(tr -d '[:space:]' <"$VERSION_FILE")"

# A floating minor silently picks up new patch releases, so the same tree
# can start failing on a day nobody touched it. Dependabot does not bump
# this value, so the pin is deliberate and has to be exact to mean anything.
if ! printf '%s' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'; then
    fail "${VERSION_FILE} holds '${version}', which is not an exact version (want e.g. v2.12.2)"
fi

for workflow in .github/workflows/*.yml; do
    [ -f "$workflow" ] || continue
    # Only a step that reads VERSION_FILE may mention golangci-lint's
    # version; any other literal version next to the linter action is a
    # second source of truth.
    if grep -n 'golangci' "$workflow" >/dev/null 2>&1; then
        if grep -nE '^\s*version:\s*v[0-9]' "$workflow" >/dev/null 2>&1; then
            fail "${workflow} hardcodes a golangci-lint version; read ${VERSION_FILE} instead"
            grep -nE '^\s*version:\s*v[0-9]' "$workflow" | sed 's/^/    /' >&2
        fi
    fi
done

# A bare `golangci-lint` invocation resolves from PATH, which is the
# divergence this whole arrangement exists to prevent.
if grep -nE '^\tgolangci-lint (run|fmt)' Makefile >/dev/null 2>&1; then
    fail "Makefile calls golangci-lint from PATH; use \$(GOLANGCI_LINT_VERSION) via go run"
    grep -nE '^\tgolangci-lint (run|fmt)' Makefile | sed 's/^/    /' >&2
fi

if [ -f lefthook.yml ] && grep -nE 'golangci-lint@v[0-9]' lefthook.yml >/dev/null 2>&1; then
    fail "lefthook.yml hardcodes a golangci-lint version; read ${VERSION_FILE} instead"
    grep -nE 'golangci-lint@v[0-9]' lefthook.yml | sed 's/^/    /' >&2
fi

if [ "$failures" -ne 0 ]; then
    echo "check-golangci-pin: ${failures} problem(s) found" >&2
    exit 1
fi

echo "check-golangci-pin: OK, golangci-lint pinned at ${version} in ${VERSION_FILE}"
