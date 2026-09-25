<!-- The merge gate checks paths and dependency files automatically. The items
     below are the part it cannot check. A reviewer will close a PR that ticks
     boxes it cannot justify. -->

## What changed and why

<!-- One or two sentences. What behavior changes for a caller or a gate? -->

## The gate will flag this PR if it touches maintainer-owned paths

Those paths are listed in `scripts/merge-gate.sh`. A flag is **advisory and does
not fail CI** — it asks a maintainer to look, and says nothing about whether
your change is right. Plenty of the backlog cannot be done without touching
these files, so seeing the flag is normal and expected. The finding appears in
the Merge gate job summary.

## Checklist

- [ ] **No new third-party dependency** (Go or Rust) was added. If one was, the
      gate holds the PR — say here why it is warranted and what trust it brings.
- [ ] **No severity threshold moved** without a reason traceable to something
      the attestation run actually surfaced (`docs/attestation-run.md`).
- [ ] **No verdict is published that the evidence does not support.** Undetermined
      stays undetermined; nothing renders "could not check" as "this is fine".
- [ ] **Tests pass locally**: `go test ./...` and `cd assay-contracts && cargo test`.
- [ ] If this PR touches a fail-closed path, **a test would fail if the
      fail-closed behavior regressed** — a fix without a regression test is a
      fix that can silently break again.
