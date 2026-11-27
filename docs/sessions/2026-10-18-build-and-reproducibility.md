# Session: build floor and reproducibility

**2026-10-06 through 2026-10-23.** October's build and release changes followed
new toolchain/security requirements while making attestation reproduction a
repeatable operational check.

## Go support moved with upstream requirements

- [PR #473](https://github.com/use-assay/Assay/pull/473) raised the Go floor to
  1.25.13 so `govulncheck` could pass.
- [PR #470](https://github.com/use-assay/Assay/pull/470) raised the floor to
  1.26.9 for the `net/http` fixes.
- [PR #521](https://github.com/use-assay/Assay/pull/521) later raised it to
  1.26.10 for GO-2026-6809.

These are deliberate minimum-version changes, not a claim that every newer Go
release has been independently certified. `go.mod` remains the source of truth
for the build floor.

## Reproducibility became an operational signal

[PR #487](https://github.com/use-assay/Assay/pull/487) added release machinery,
reproducible binaries, the recorded preimage version, and scheduled-failure
tracking. The weekly live check is intentionally separate from pull-request CI:
it depends on public ledgers and issuer services, so an outage is not a code
regression.

[PR #546](https://github.com/use-assay/Assay/pull/546) made evidence hashing
deterministic when a source fetch fails and closed the underlying #24 issue.
That does not authorize replacing an on-chain hash with a newly observed one:
evidence drift still requires inspection and, where appropriate, a reviewed
re-attestation. The recurring scheduled alert in [#548](https://github.com/use-assay/Assay/issues/548)
must be classified from its run output; an inconclusive scan is not a pass and
must not be converted into an attestation.
