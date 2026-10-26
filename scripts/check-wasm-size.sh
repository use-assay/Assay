#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd)
BASELINE=${WASM_SIZE_BASELINE:-$REPO_ROOT/assay-contracts/wasm-size-baseline.json}
RECORD=0

if [ "${1:-}" = "--record" ]; then
  RECORD=1
elif [ $# -gt 0 ]; then
  echo "usage: $0 [--record]" >&2
  exit 2
fi

if ! command -v stellar >/dev/null 2>&1; then
  echo "check-wasm-size: stellar CLI is required" >&2
  exit 2
fi
if [ ! -f "$BASELINE" ]; then
  echo "check-wasm-size: baseline not found: $BASELINE" >&2
  exit 2
fi

read -r baseline_cli tolerance < <(python3 - "$BASELINE" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as fh:
    data = json.load(fh)
print(data["stellar_cli"], data.get("tolerance_percent", 10))
PY
)

actual_cli=$(stellar --version | awk 'NR == 1 { print $2 }')
if [ "$RECORD" -eq 0 ] && [ "$actual_cli" != "$baseline_cli" ]; then
  echo "check-wasm-size: baseline was recorded with stellar $baseline_cli, found $actual_cli" >&2
  echo "check-wasm-size: sizes from different CLI versions are not comparable" >&2
  exit 2
fi

OUT_DIR=$(mktemp -d)
trap 'rm -rf "$OUT_DIR"' EXIT

cd "$REPO_ROOT/assay-contracts"
BUILD_LOG=$OUT_DIR/build.log
if ! stellar contract build --manifest-path Cargo.toml \
  --package assay-safety-registry --out-dir "$OUT_DIR" >>"$BUILD_LOG" 2>&1; then
  cat "$BUILD_LOG" >&2
  exit 2
fi
if ! stellar contract build --manifest-path Cargo.toml \
  --package assay-example-gate --out-dir "$OUT_DIR" >>"$BUILD_LOG" 2>&1; then
  cat "$BUILD_LOG" >&2
  exit 2
fi

python3 - "$BASELINE" "$OUT_DIR" "$tolerance" "$RECORD" "$actual_cli" <<'PY'
import json
import os
import sys

baseline_path, out_dir, tolerance, record, cli = sys.argv[1:6]
tolerance = int(tolerance)
record = record == "1"

contracts = {
    "assay_safety_registry": "assay_safety_registry.wasm",
    "assay_example_gate": "assay_example_gate.wasm",
}

with open(baseline_path, encoding="utf-8") as fh:
    baseline = json.load(fh)

sizes = {}
for name, filename in contracts.items():
    path = os.path.join(out_dir, filename)
    if not os.path.exists(path):
        sys.stderr.write("check-wasm-size: build did not produce %s\n" % path)
        sys.exit(2)
    sizes[name] = os.path.getsize(path)

if record:
    data = {
        "stellar_cli": cli,
        "tolerance_percent": tolerance,
        "contracts": sizes,
    }
    with open(baseline_path, "w", encoding="utf-8") as fh:
        json.dump(data, fh, indent=2, sort_keys=True)
        fh.write("\n")
    print("recorded %s" % baseline_path)
    for name, size in sorted(sizes.items()):
        print("  %-24s %8d bytes" % (name, size))
    sys.exit(0)

recorded = baseline.get("contracts", {})
failed = False
print("%-24s %10s %10s %9s %10s  %s" % ("contract", "baseline", "current", "delta", "limit", "status"))
for name in sorted(contracts):
    if name not in recorded:
        sys.stderr.write("check-wasm-size: baseline has no size for %s\n" % name)
        sys.exit(2)
    base = int(recorded[name])
    size = sizes[name]
    limit = base + base * tolerance // 100
    delta = size - base
    status = "ok" if size <= limit else "REGRESSION"
    if size > limit:
        failed = True
    print("%-24s %10d %10d %+9d %10d  %s" % (name, base, size, delta, limit, status))

if failed:
    print("wasm size exceeded the %d%% tolerance; update the baseline only if the growth is intended" % tolerance)
    sys.exit(1)
PY
