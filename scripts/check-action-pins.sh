#!/usr/bin/env bash
# Fails if a GitHub Actions workflow references an action by a floating tag
# instead of a pinned commit SHA.
#
# A tag like @v7 is a mutable pointer: whoever controls the action repo can
# move it to a different commit at any time, and every subsequent run of a
# workflow that has not itself changed picks up whatever it now points at.
# Some of these workflows hold contents: write and run on every push, so a
# moved tag is a supply-chain path straight into release artifacts.
set -euo pipefail

failures=0

fail() {
    echo "check-action-pins: $1" >&2
    failures=$((failures + 1))
}

for workflow in .github/workflows/*.yml; do
    [ -f "$workflow" ] || continue

    # A pinned ref is `uses: owner/repo@<40-char sha>`. Anything else after
    # `uses:` (a tag, a branch, a short sha) is floating.
    while IFS=: read -r lineno line; do
        ref="$(printf '%s' "$line" | sed -nE 's/.*uses:[[:space:]]*[^@]+@([^[:space:]]+).*/\1/p')"
        [ -n "$ref" ] || continue
        if ! printf '%s' "$ref" | grep -Eq '^[0-9a-f]{40}$'; then
            fail "${workflow}:${lineno} uses an unpinned ref '${ref}'"
        fi
    done < <(grep -n 'uses:' "$workflow")

    # The version comment is not decoration: Dependabot's github-actions
    # ecosystem reads it to know which release a SHA corresponds to. Without
    # it, the pin stops receiving updates and quietly rots.
    while IFS=: read -r lineno line; do
        if printf '%s' "$line" | grep -Eq '@[0-9a-f]{40}[[:space:]]*$'; then
            fail "${workflow}:${lineno} pins a SHA with no trailing '# vN' comment"
        fi
    done < <(grep -n 'uses:.*@[0-9a-f]\{40\}' "$workflow")
done

if [ "$failures" -ne 0 ]; then
    echo "check-action-pins: ${failures} problem(s) found" >&2
    exit 1
fi

echo "check-action-pins: OK, every workflow action reference is SHA-pinned"
