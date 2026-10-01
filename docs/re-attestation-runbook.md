# Re-attestation runbook

An attestation is a snapshot with an `attested_at` timestamp. Nothing in Assay
refreshes it, and the registry has no scheduler. This runbook is the procedure
for keeping the attested corpus current, and the reasoning for why it is manual
today.

Scope: the ten attested assets recorded in
[deployment.md](deployment.md#attested-assets) on the testnet registry
`CBK4FBIH…`. Related: [freshness.md](freshness.md),
[attester-key.md](attester-key.md), [deployment.md](deployment.md).

## Why this exists

Every attestation in the registry is older than the example gate's 24-hour
freshness window, so the canonical gate refuses assets whose data is correct.
That is the freshness policy working, not a defect: a gate with a strict window
and no refresher refuses everything. The remedy is re-attestation, and this
document is the procedure.

## Cadence and the freshness window

The re-attestation cadence must be **shorter than the shortest freshness window
any consumer enforces**, otherwise consumers see the corpus go stale before it
is refreshed. The example gate uses 24 hours. The recommended windows in
[freshness.md](freshness.md#recommended-windows) are 24 hours for a custodial
deposit gate and shorter for one-shot settlement, so the binding constraint is
the strictest consumer, not the median one.

| Consumer class | Window | Implied minimum cadence |
| --- | --- | --- |
| Custodial deposit gate | 24 h | Daily (or better) |
| One-shot settlement | <= 1 h, or a fresh scan at execution | Per-settlement re-scan, not a schedule |
| Monitoring / dashboards | 7 d | Weekly |
| Explorer badge | 30 d | Monthly |

A single corpus re-attestation refreshes `attested_at` for every asset at once.
Running it at the strictest consumer's window (daily, for the example gate) is
the conservative default. Re-attesting **also renews each entry's TTL**
([deployment.md](deployment.md#what-happens-as-an-entry-ages)), so freshness and
retention share one loop; there is deliberately no second scheduler.

## The corpus

The ten assets and their issuers are the table in
[deployment.md](deployment.md#attested-assets). The primary verification pair
from the issue is:

```sh
make attest ASSET=AQUA-GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA
make read   ASSET=AQUA-GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA
```

## Procedure (manual)

Run these per asset. Do them one at a time so a failure stops at a known place
rather than part-way through a batch.

1. **Scan and write** with the live-scan path. `make attest` derives severity,
   the mechanics bitset, and `evidence_hash` from a fresh scan and submits them;
   no hand-written value can reach the contract.

   ```sh
   make attest ASSET=CODE-ISSUER
   ```

2. **Read it back** and confirm severity, flags and `evidence_hash` match the
   pre-write values. A re-attestation of unchanged sources must reproduce the
   same `evidence_hash` (it commits to the claims, not the clock); only
   `attested_at` moves.

   ```sh
   make read ASSET=CODE-ISSUER
   ```

3. **Record the run** (see [Recording a run](#recording-a-run)) and, if an
   asset's verdict moved, add the asset and the reason to
   [attestation-run.md](attestation-run.md) rather than silently accepting the
   change.

## Safety rules (not negotiable)

These are enforced in code and must not be bypassed by any automation:

- **An undetermined scan is never attested.** `attest.FromReport` refuses an
  undetermined report. A check that could not conclude ("we could not read the
  source") is not the same as "clean", and a partial scan must not become an
  on-chain statement: `get_safety` has nowhere to carry the caveat.
- **A stale verdict is never attested as fresh.** `attest.FromReport` returns
  `ErrStale` when the report is outside its policy window, so a cached or
  mis-timed report cannot be written as current.
- **A source outage is retried later, not worked around.** If a consumed source
  is down, the scan is undetermined and the write is skipped, not forced. See
  [source-outage-runbook.md](source-outage-runbook.md).
- **Never pass `max_age_secs = 0` to make a read stop failing.** That disables
  the freshness check; it is a property of the query, not a production policy.

## Key custody: why this is manual today

`make attest` signs as the single `assay-attester` key, whose seed lives on one
machine. There is no threshold, no multisig, and no unattended signing
authority, so **unattended automation is deliberately not enabled**: a scheduler
that can sign without an operator is a single-machine key with a cron job
attached, which widens the compromise window without improving what the key can
do ([attester-key.md](attester-key.md#custody-and-backup--current-state-stated-plainly)).

The safety properties above are what gating automation on custody trades
against. When key custody is answered (a threshold admin, per
[multi-attestor.md](multi-attestor.md#recommendation)), the same procedure can
be driven on a schedule without weakening the undetermined/stale refusals,
because those live in `attest.FromReport`, below the scheduler.

## Recording a run

Every run is recorded, including runs that skip assets.

| Date | Assets attempted | Written (`attested_at`) | Skipped (reason) | Notes |
| --- | --- | --- | --- | --- |
| 2026-09-16 | AQUA, DOGE | AQUA `1789546357`, DOGE `1789546342` | — | #26 fix verification; hashes unchanged ([deployment.md](deployment.md#the-26-fix-before-and-after)) |

A full-corpus re-attestation has **not** been run since the corpus was written
(2026-08-15 to 2026-09-17); the entry above is the last recorded re-attestation
and covers two assets, not all ten. Running and recording a full pass is the
outstanding work for this issue. Record the result here (and any verdict
movement in [attestation-run.md](attestation-run.md)) when it happens.

## Semantics

- **Valid state** - Scan completes and the attestation is written; `attested_at`
  advances and, if the sources were unchanged, `evidence_hash` is identical.
- **Unknown state** - Scan undetermined: skip the asset and record why. Do not
  write. Enforced by `attest.FromReport`.
- **Unavailable state** - Source outage: retry later; never attest a partial
  result ([source-outage-runbook.md](source-outage-runbook.md)).
- **Stale state** - An asset past a consumer's window is the **trigger** for
  re-attestation, not an error.
