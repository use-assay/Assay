# Rollback procedure for a wrong attestation

Assay has already produced verifiable-but-wrong reports twice. Neither was
attested, but if either had been, there was no written answer to what happens
next. This is that answer, and it is honest about the fact that the corrective
paths differ depending on whether revocation exists.

Scope: the testnet deployment. Related:
[verifying.md](verifying.md), [SECURITY.md](../SECURITY.md),
[attester-key.md](attester-key.md),
[deployment.md](deployment.md#migrating-to-a-registry-with-revoke).

## How a wrong attestation is identified, and by whom

An attestation is a set of claims (`severity`, `flags`, `evidence_hash`) about a
moment. It is wrong when a fresh scan of the same asset, using the same check
set, does not reproduce it and the difference is **not** explained by the asset
actually having changed. Identification happens by:

- **Re-scanning**, per [verifying.md](verifying.md): recompute severity, flags
  and `evidence_hash` and compare against the on-chain values. Anyone can do
  this; it needs no privileged access.
- **A scanner-bug report**, filed through the security-adjacent path in
  [SECURITY.md](../SECURITY.md), when the divergence is traced to a defect in
  Assay rather than to the asset.

The two precedents in this repository were scanner bugs: verifiable reports that
were still wrong. The hash mismatch alone is *not* conclusive about cause - it
can be a real asset change, a scanner downgrade, or the machine-dependent
transport-text caveat documented in [attester-key.md](attester-key.md#can-do) -
so a human must attribute it before acting.

## Immediate corrective action available today

**Overwrite with a correct attestation.**

```sh
# after establishing, by fresh scan, that the on-chain value is wrong:
make attest ASSET=CODE-ISSUER   # writes the corrected severity/flags/evidence_hash
make read   ASSET=CODE-ISSUER   # confirm the on-chain values now match the scan
```

`attest` overwrites the asset's entry. There is no per-asset history in the
contract; the previous values are gone from on-chain state.

**Revocation does not exist on the live registry.** `revoke` (which removes an
entry, after which `get_safety` returns `None` and gates fail closed) ships in
new contract code, but the registry at `CBK4FBIH…` has no upgrade entrypoint and
cannot pick it up. Until the migration in
[deployment.md](deployment.md#migrating-to-a-registry-with-revoke) is done,
overwrite is the only corrective action. **Update this procedure if revocation
lands** - it changes the immediate action for the invalid state from "overwrite"
to "overwrite or revoke, by choice."

## Preserving the historical record

The correction must not erase what happened. The on-chain value can be
overwritten, but the record of the mistake is kept off-chain, following the
pattern already used for the duplicate gate instance and the superseded gate in
[deployment.md](deployment.md#deployment-transactions):

1. **Record the wrong attestation**: the asset, the wrong `severity`/`flags`/
   `evidence_hash`, its `attested_at`, and its attest transaction hash, before
   overwriting. This is the only durable copy of what was said.
2. **Record why it was wrong**: the fresh scan that contradicted it, and the
   attribution (asset change, scanner bug, toolchain) with its evidence.
3. **Record the correction**: the new values, the overwrite transaction, and
   the date.
4. **Add it to [attestation-run.md](attestation-run.md)** if a verdict moved, in
   the same style as the existing findings, so the run's narrative stays
   complete.

Two properties help a third party reconstruct this even without Assay's records:
the wrong attestation's `evidence_hash` was checkable by anyone who read it
before the overwrite, and Soroban `attest` events give an indexer both the write
and its replacement. On-chain state alone, however, shows only the latest value;
the honest record is the off-chain one.

## Informing consumers

- **Immediately** tell integrators to treat the asset's attestation as
  untrusted until the correction is written, and to fail closed (do not pass
  `max_age_secs = 0` to keep reading it).
- A **fresh false** attestation is not caught by any freshness window: a
  consumer that gates only on severity and age would admit on it. Notification
  is therefore the control, not time.
- **Re-attest first, then correct the record.** If an actual asset change is the
  cause, the corrected attestation is simply the next run; if a scanner bug is
  the cause, disclose it via [SECURITY.md](../SECURITY.md) so other consumers of
  the same scanner are warned.

## Disputed correctness (`unknown`)

When correctness is disputed - a party claims the attestation is wrong and a
re-scan disagrees - the decision is made on **evidence, by the maintainers**,
using the same standard the project applies everywhere: no verdict is published
that the evidence does not support
([CONTRIBUTING.md](../CONTRIBUTING.md)). Until it is resolved, the entry is left
as written and flagged in the record; it is not silently changed to a third
value that no scan produced.

## Semantics

- **Valid state** - Attestation correct: no action.
- **Invalid state** - Attestation wrong: overwrite (or revoke, once it exists),
  record what it said and why it was wrong, and inform consumers.
- **Unknown state** - Correctness disputed: the maintainers decide on evidence;
  the record states who decided and on what.

## Out of scope

Not implementing revocation - that is the contract migration in
[deployment.md](deployment.md#migrating-to-a-registry-with-revoke). Not
changing the freshness policy. This document is procedural.
