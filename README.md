# Assay

Mechanical trap-scanner and on-chain safety gate for Stellar assets.

Assay answers one question: **can the issuer of this asset take or freeze my
tokens after I hold them?** It answers it from the ledger's own rules — the
issuer's authorization flags — not from reputation, community reports, or a
domain blocklist.

## Why this exists

Stellar already has a reputation layer, and Assay does not rebuild it.
StellarExpert publishes asset ratings, a curated address directory, and a
malicious-domain blocklist; SEP-0037 defines the directory format; the SCF
runs a scam-flagging process. Assay **consumes** those as attributed input
signals and never re-derives them.

What no tool in the ecosystem does is the other half:

1. **Trap mechanics from the ledger itself.** Authorization flags are
   consensus-enforced facts about what the issuer is *able* to do. They are
   true whether or not anyone has reported the asset yet, which means a
   brand-new asset with no reputation at all is still fully classifiable.
2. **An on-chain gate.** `get_safety(asset)` is designed to be callable by
   another Soroban contract, atomically, in the same transaction as the
   action it protects.

## The hard part

Detecting a flag is trivial. Judging it is not.

`auth_clawback_enabled` is not inherently malicious — it is the mechanism
regulated issuers use to meet legal obligations like sanctions enforcement
and court-ordered reversal. A scanner that paints every clawback-capable
asset red is wrong about every well-run regulated stablecoin, and a scanner
that is wrong about the biggest assets on the network gets ignored.

So Assay's engine reports **severity plus reasoning**, never a boolean, and
carries an explicit, evaluated position on legitimate clawback use. See
[docs/severity-model.md](docs/severity-model.md) for the model and
[docs/eval.md](docs/eval.md) for the labelled set it is measured against.

## Docs

| Document | What's in it |
| --- | --- |
| [docs/severity-model.md](docs/severity-model.md) | The judgment layer: severity levels, the legitimate-use carve-out, and why |
| [docs/checks.md](docs/checks.md) | Each mechanic Assay checks, and what it can and cannot conclude |
| [docs/contract-interface.md](docs/contract-interface.md) | `get_safety(asset)` design |
| [docs/eval.md](docs/eval.md) | Labelled trap/legitimate set and current results |
| [docs/adding-a-check.md](docs/adding-a-check.md) | How to write a new mechanic check |

## Status

Early. The mechanics engine and HTTP API are usable; the Soroban contract is
an interface stub, not a deployed gate.

## License

Apache-2.0. See [LICENSE](LICENSE).
