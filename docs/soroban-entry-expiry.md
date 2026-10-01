# Soroban entry expiry and its operational consequence

Expiry is a scheduled, silent loss of every attestation. Its effect - gates
failing closed - is safe, so nobody notices until someone asks why nothing is
admitted. This document states the mechanism, how to observe remaining lifetime
(or that you cannot), what happens when an entry lapses, and how this relates to
re-attestation.

Scope: the testnet deployment. The authoritative measurements live in
[deployment.md](deployment.md#entry-lifetime); this page is the operator-facing
summary. Related: [re-attestation-runbook.md](re-attestation-runbook.md),
[freshness.md](freshness.md).

## Mechanism and observed parameters

Soroban persistent entries have a time-to-live measured in **ledgers**, set by
network configuration (validator vote), not by the contract. When it lapses the
entry is **archived**.

Measured on **2026-09-27** with `stellar network settings --network testnet`
(protocol 28):

| Setting | Ledgers | At ~5 s per ledger |
| --- | --- | --- |
| `min_persistent_ttl` (what a fresh write gets) | 120,960 | ~7 days |
| `max_entry_ttl` (the most an extension can grant) | 3,110,400 | ~180 days |

The contract extends TTLs on **write** (`init`, `attest`, `attest_many`,
`revoke`), to `env.storage().max_ttl()` - the host maximum, not a hard-coded
constant - and never on read. Retention therefore follows the writer: keeping an
attestation live is the attester's decision, made by re-attesting.

## How to check remaining lifetime

Yes, but not from the contract ABI. There is no entrypoint that returns an
entry's remaining TTL; read it from the ledger:

```sh
# liveUntilLedgerSeq is the ledger at which the entry will be archived.
# 0 (or a value in the past) means it is already archived.
scripts/check-deployment.sh          # reads chain state via one getLedgerEntries call
```

`getLedgerEntries` returns each entry's `liveUntilLedgerSeq`. To extend one entry
by hand (needs no contract entrypoint):

```sh
KEY=$(echo '{"vec":[{"symbol":"Safety"},{"address":"<SAC>"}]}' | stellar xdr encode --type ScVal)
stellar contract extend --id CBK4FBIH… --key-xdr "$KEY" --durability persistent \
  --ledgers-to-extend 3110399 --source-account assay-attester --network testnet
```

The live registry is **already archived**: as of 2026-09-27, five sampled
attestations plus the contract instance and contract code all reported
`liveUntilLedgerSeq: 0`. The deployment predates TTL handling and nothing ever
extended it.

## Effect when an entry lapses

An archived entry is **not lost**, and it is **not** indistinguishable from
never-attested:

- **It restores on access.** Since protocol 23, an archived persistent entry is
  restored automatically when a transaction's footprint marks it for restoration,
  and simulation adds that marking on its own. A simulated `get_safety` on an
  archived attestation returns the stored value with its **original
  `attested_at`**; the first transaction that touches it pays the restore fee.
- **Archival is not expiry.** It never makes an attestation vanish, and it never
  makes one read as fresh - the original `attested_at` survives. What stops a
  gate admitting a months-old attestation is `max_age_secs`, not archival. A
  caller passing `max_age_secs = 0` is served the restored value as-is.
- **A gate with a normal window fails closed.** Any window under ~180 days
  refuses a restored-after-archival attestation as stale, which is safe but reads
  to a consumer much like never-attested. To make an attestation unreadable on
  purpose, `revoke` it (once available); do not wait for archival, because
  archival does not do that.

Reads never extend anything, so a gate that keeps reading an archived entry keeps
paying the restore fee.

## Relationship to re-attestation

Re-attesting pushes the entry's TTL back out to the network maximum (the
`reattest_renews_ttl` test pins this), so keeping entries live is a **by-product
of keeping them fresh**. There is no separate TTL-maintenance schedule: the
re-attestation loop in [re-attestation-runbook.md](re-attestation-runbook.md),
at the cadence set by the shortest consumer window in
[freshness.md](freshness.md#recommended-windows), is the same loop that keeps
entries live. Every recommended freshness window is 30 days or shorter, well
under the ~180-day maximum TTL, so for any gating use an attestation goes stale
long before it is archived.

An asset dropped from the loop simply ages: live for ~180 days, then archived
and restored-on-read with its original timestamp.

## Semantics

- **Valid state** - Entry live (`liveUntilLedgerSeq` in the future): normal
  reads.
- **Stale state** - Entry archived: gates fail closed on `max_age_secs`, which
  is safe; a `max_age_secs = 0` caller still gets the restored original value.
- **Unknown state** - If `liveUntilLedgerSeq` cannot be read (RPC unreachable),
  say so rather than implying lifetime can be observed. The value is readable
  with current tooling; an unread result means the node did not answer, not that
  the entry has no TTL.

## Out of scope

Not implementing TTL extension beyond the extend-on-write policy already in the
contract (TTL handling, #87). Not defining a second schedule; re-attestation
(#145) is the schedule.
