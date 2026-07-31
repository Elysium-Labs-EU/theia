#!/usr/bin/env bash
# Blocks a PR diff from landing an accidental large or binary blob.
#
# theia has no Git LFS setup, so any file that shows up as an LFS pointer
# ("version https://git-lfs.github.com/spec/...") got there by an author's
# local LFS config leaking into the commit, not a deliberate choice — reject
# it same as an oversized file.
#
#   FILE_SIZE_BASE       base ref (default: origin/main); CI sets it to PR target
#   FILE_SIZE_MAX_BYTES  per-file cap (default: 1048576, i.e. 1 MiB)
set -euo pipefail

BASE="${FILE_SIZE_BASE:-origin/main}"
MAX_BYTES="${FILE_SIZE_MAX_BYTES:-1048576}"

if ! git rev-parse --verify --quiet "$BASE" >/dev/null 2>&1; then
  git fetch --quiet origin "${BASE#origin/}" 2>/dev/null || true
fi
if git rev-parse --verify --quiet "$BASE" >/dev/null 2>&1; then
  DIFF_BASE="$(git merge-base "$BASE" HEAD 2>/dev/null || echo "$BASE")"
else
  echo "check-file-size: base ref '$BASE' unresolvable; nothing to compare against, passing." >&2
  exit 0
fi

CHANGED="$(git diff --name-only --diff-filter=ACM "$DIFF_BASE" HEAD || true)"
if [ -z "$CHANGED" ]; then
  echo "check-file-size: no added/modified files vs $BASE; nothing to gate."
  exit 0
fi

failed=0
while IFS= read -r f; do
  [ -f "$f" ] || continue

  size=$(wc -c < "$f" | tr -d ' ')
  if [ "$size" -gt "$MAX_BYTES" ]; then
    echo "check-file-size: $f is $size bytes, exceeds $MAX_BYTES byte cap" >&2
    failed=1
  fi

  if head -c 200 "$f" | grep -q '^version https://git-lfs\.github\.com/spec/'; then
    echo "check-file-size: $f is a Git LFS pointer; theia does not use LFS" >&2
    failed=1
  fi
done <<< "$CHANGED"

if [ "$failed" -ne 0 ]; then
  exit 1
fi
echo "check-file-size: OK, no changed file vs $BASE exceeds $MAX_BYTES bytes or is an LFS pointer."
