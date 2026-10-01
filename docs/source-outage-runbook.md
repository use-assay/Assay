# Source-outage runbook

Assay's correct behaviour under a source outage is already implemented and
tested. What was missing is a document telling an operator what to do when one
happens. This is that document.

Scope: the testnet deployment. Related:
[checks.md](checks.md#reputation),
[re-attestation-runbook.md](re-attestation-runbook.md),
[deployment.md](deployment.md).

## The two outage classes

The response differs by class, and confusing them is the mistake this runbook
exists to prevent.

| Class | Source | Code path | Effect |
| --- | --- | --- | --- |
| **Consumed** | StellarExpert (directory, blocklist), SEP-1 `stellar.toml`, SEP-0042 asset lists | A check records the failure as attributed evidence and marks the finding `undetermined` | Scan **completes as undetermined**; attestation is **blocked** |
| **Ledger** | Horizon (issuer flags, accounts, assets) | `internal/scan` treats the failure as **fatal** | Scanning **stops entirely** |

The asymmetry is deliberate. A consumed source that did not answer means a
question Assay wanted to ask could not be put; the honest result is
`undetermined`, and `attest.FromReport` refuses to write it. Horizon is the
source every severity is derived from, so if it cannot be read there is nothing
to scan.

## Recognising each

- **Consumed outage.** The scanner finishes and the report names the check on
  `undetermined_checks`, with the source's failure recorded verbatim as
  evidence (HTTP 429/5xx, timeout, DNS failure). A 404 is *not* an outage: it
  means the source was read and had no entry.
- **Ledger outage.** The scan aborts with an error and no report is produced,
  e.g. `assay: horizon: not found: /accounts/…` when an endpoint disagrees with
  itself, or a transport error when Horizon is unreachable.

Both classes have been observed live: StellarExpert rate-limited during a
10-asset sweep on 2026-09-05, and a DNS failure affected a scan on 2026-09-22.

## Effect, precisely

- **On scans.** Consumed: the scan completes, marked undetermined, naming the
  failed check. Ledger: the scan stops; no verdict is produced.
- **On attestation.** Consumed (undetermined): blocked, because a partial scan
  must not become a statement `get_safety` cannot qualify. Ledger (no report):
  blocked for the same reason, one step earlier.
- **On already-written attestations.** Nothing. An outage cannot change an
  on-chain attestation or make it read differently. It only prevents *new*
  writes and re-attestations; existing entries keep their `attested_at`, and
  whether a consumer accepts them is the consumer's `max_age_secs` window.

## What to do

1. **Confirm the class** from the scanner output: `undetermined_checks` names a
   consumed source; an aborted scan with no report is the ledger.
2. **Wait and retry later.** Both classes are transient by nature. There is no
   retry loop today; re-run the scan when the source is back.
3. **Re-attest once the source answers** as part of the normal cadence; the
   outage may simply delay that day's run. The procedure and the recording
   table are in [re-attestation-runbook.md](re-attestation-runbook.md).
4. **Record the outage** if it delayed a run or changed a verdict, so a later
   reader can tell an asset change from a source change.

## What NOT to do

- **Do not disable a check to get a scan through.** A scan with a check turned
  off is not a weaker scan, it is check suppression: it produces a verdict the
  evidence does not support, and it will be written as if the source had agreed.
- **Do not hand-write a severity or an `evidence_hash`** to replace a failed
  scan. `make attest` derives every submitted value from a live scan precisely so
  this cannot happen.
- **Do not pass `max_age_secs = 0` to make a read succeed** during an outage.
  That disables freshness; it does not fix the source and it hides the problem.
- **Do not treat an unanswered source as a clean one.** "We could not read it"
  and "it is fine" must never render the same; the scanner already distinguishes
  them and this runbook keeps them distinguished.

## Semantics

- **Valid state** - All sources reachable; normal operation.
- **Unavailable state** - A consumed source is down: scans undetermined,
  attestation blocked, existing attestations unaffected.
- **Invalid state** - The ledger source is down: scanning stops entirely.

See [re-attestation-runbook.md](re-attestation-runbook.md) for the run that
follows an outage.
