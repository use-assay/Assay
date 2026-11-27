#!/usr/bin/env bash
# Fails if any workflow step references an action by anything other than a
# full 40-hex commit SHA. Tags and branches are mutable upstream; a SHA is not.
# Local actions (./path) and docker:// images are not GitHub action refs and
# are skipped. See CONTRIBUTING.md, "Dependency policy".
set -eu

cd "$(dirname "$0")/.."

# Match only the `uses:` key of a step (optionally after the list dash), so a
# permission such as `statuses: read` is not mistaken for one.
unpinned=$(grep -rnE '^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]' .github/workflows \
    | grep -vE 'uses:[[:space:]]+(\./|docker://)' \
    | grep -vE 'uses:[[:space:]]+[^[:space:]@]+@[0-9a-f]{40}([[:space:]]|$)' || true)

if [ -n "$unpinned" ]; then
    echo "these action references are not pinned to a full commit SHA:" >&2
    echo "$unpinned" >&2
    echo "pin each as owner/repo@<40-hex sha> # vX.Y.Z" >&2
    exit 1
fi
echo "all action references are pinned to commit SHAs"
