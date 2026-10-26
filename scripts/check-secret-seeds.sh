#!/usr/bin/env bash
set -euo pipefail

if [ $# -lt 1 ] || [ $# -gt 2 ]; then
  echo "usage: $0 <base-ref> [head-ref]" >&2
  exit 2
fi

BASE=$1
HEAD=${2:-HEAD}
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd)
cd "$REPO_ROOT"

TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT

git diff --unified=0 --no-color --no-ext-diff --no-textconv "$BASE...$HEAD" -- . > "$TMP"
go run ./scripts/secretscan -diff < "$TMP"
