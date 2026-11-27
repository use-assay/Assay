# Pubnet readiness checklist

Assay is deployed on testnet only. [threat-model.md](threat-model.md) says
testnet is **not a candidate for mainnet**, and the reasons are spread across
[threat-model.md](threat-model.md), [deployment.md](deployment.md) and
[multi-attestor.md](multi-attestor.md). This page consolidates them into one
list so the gap is measurable and maintainers have a single source of truth.

A condition is **not** satisfied by code that exists only in this repository.
The live testnet registry predates several of these features and has no upgrade
entrypoint, so deploying current source is itself a prerequisite; the migration
is tracked separately. Status below distinguishes "in the repository" from
"on the live registry."

## Checklist

| # | Condition | Status | Tracking issue |
| --- | --- | --- | --- |
| 1 | **Threshold attestation** — more than one attestor must agree before an admission, so one compromised key cannot write any verdict | Source only: quorum reads exist, but no entrypoint writes multi-attestor records and the live registry is single-admin | [#88](https://github.com/use-assay/Assay/issues/88) |
| 2 | **Live revoke** — a wrong attestation can be withdrawn and indexers see the withdrawal, not only an overwrite | Source only: `revoke` is implemented and tested, but the live registry predates it and cannot be upgraded | [#86](https://github.com/use-assay/Assay/issues/86) |
| 3 | **Rotation** — the attester/admin key can be handed over without redeploying and re-attesting the corpus | Source only: two-step `transfer_admin`/`accept_admin` is implemented and tested; the live registry predates it | [#89](https://github.com/use-assay/Assay/issues/89) |
| 4 | **Re-attestation automation** — attestations stay inside a known freshness window without a human | Done: runbook and scheduler semantics are documented and the freshness window is measured | [#145](https://github.com/use-assay/Assay/issues/145) |
| 5 | **Key custody** — who holds the attester key, where it lives, and what happens on loss or compromise | Done: custody, loss and compromise response, and rotation limits are documented | [#150](https://github.com/use-assay/Assay/issues/150) |
| 6 | **Reproducible scanner** — a third party can rebuild the binary and reproduce the evidence hashes bit-for-bit | Done: reproducible build and rebuild procedure are documented | [#281](https://github.com/use-assay/Assay/issues/281) |
| 7 | **Audit** — every write and withdrawal has an on-chain trail an indexer can reconstruct | Source only: `attest` events carry the full stored record and `revoke` events exist; the live registry predates them | [#92](https://github.com/use-assay/Assay/issues/92) |

## Reading the statuses

- **Done** means the condition is satisfied in the repository and by the
  operating procedure, not that it has been exercised on pubnet.
- **Source only** means the code and tests exist but the deployed registry does
  not contain the feature. The deployed testnet contract predates every
  lifecycle entrypoint and has no upgrade path.
- **Partial** means the mechanism is present but does not yet cover the whole
  condition.
- **Not ready** means an admission can still be produced without the condition
  holding, so a pubnet deployment would carry that risk unmitigated.

Do not mark a condition done because its issue is closed: check that the
condition holds against the registry that would serve pubnet traffic.

## Related reading

- [deployment.md](deployment.md) — deployed addresses and the live registry's
  limitations.
- [threat-model.md](threat-model.md) — actors, defended attacks, and accepted
  risks.
- [multi-attestor.md](multi-attestor.md) — why threshold attestation is the
  largest remaining trust assumption.
- [reattestation.md](reattestation.md) — how the corpus is refreshed.
- [attester-key.md](attester-key.md) — key custody and rotation.
