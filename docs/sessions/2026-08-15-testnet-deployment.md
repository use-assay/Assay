# Session: testnet deployment

**2026-08-15.** The contract went from passing local tests to live on testnet
with four real assets attested and the round trip reproducible.

## Shipped

- **Registry deployed to testnet:**
  `CBK4FBIHMDTXCUPE4E3ZDVSFJSCY5FJETTKNIQPN4LFJIKKIBLKIXQ73`, wasm
  `c4105b91…dd301b`, admin `GALIEUOB…RFNNMVY`. Addresses and transaction hashes
  in [deployment.md](../deployment.md).
- **`internal/attest`** — specifies and computes `evidence_hash`, and derives the
  attest() arguments from a scan report. Closes the "no canonical encoding" gap.
- **`assay attestation CODE-ISSUER`** — prints the on-chain arguments, with
  `-preimage` for the exact bytes hashed and `-raw` for scripting.
- **Makefile: `build-contract`, `deploy-testnet`, `attest`, `read`.**
- **`contracts/example-gate`** — the integration guide as a compiling, tested
  crate, deployed to `CANO57JR…VZA44W3U` and wired to the live registry.
- **[integrating.md](../integrating.md)** and **[deployment.md](../deployment.md)**.

## Attested, from live scans

| Asset | Severity | Flags |
| --- | --- | --- |
| `AQUA` | 0 clear | `0` |
| `USDC` | 2 medium | `18` |
| `USDZ` | 3 high | `6` |
| `BERKSHIRE` | 4 critical | `54` |

Every value came out of `assay attestation`; `make attest` has no path that lets
a hand-written severity reach the contract.

## Findings

**A legitimately clawback-enabled asset exists, and it is verified.** `USDZ`
(`GAKTLPC4…OATF5U6XPR`) carries `auth_revocable | auth_clawback_enabled` and
passes reciprocal SEP-1 verification. Found by pulling StellarExpert's top 50 by
rating and checking Horizon flags for each: of the 49 issued assets in that set
(XLM has no issuer), five are clawback-capable — `ZARZ`, `USDZ`, `PYUSD`,
`USDY`, `GYEN`. All five scan `high`, none is escalated, and four of the five
verify their domain (`PYUSD` does not). This is exactly the subject the previous
session recorded as
missing from the eval — the case where the severity model's claim to treat
legitimate clawback fairly can actually be measured rather than argued.

**The bitset and the severity number disagree usefully.** `USDC` attests at
severity 2 and the registry's `is_safe(asset, 2, 0)` admits it, while the
example gate refuses it — the gate reads the `auth_revocable` bit, and a
custodial balance cannot survive a freeze. Severity answers "how bad", the
bitset answers "which power". The integration guide leads with the bitset
because of this.

**Contract IDs are network-scoped, so a mainnet asset has a testnet address.**
Attestations here are keyed on the *testnet* SAC address of assets that live on
pubnet. Scanning reads pubnet, attesting writes testnet. Recorded plainly in
deployment.md rather than glossed, because it is the kind of detail that makes a
demo look more portable than it is.

**Doc comments are part of the contract spec.** Fixing a stale doc comment in
`attest()` changed the wasm hash, so the first deployment was thrown away and
redeployed to keep the on-chain wasm matching committed source.

## Decisions

- **`evidence_hash` excludes retrieval timestamps.** Hashing them would give
  unchanged evidence a new hash every scan, making the field unverifiable by
  anyone not present for the original fetch. Excluding them means a verifier
  re-scans and reproduces the hash exactly. The hash commits to the claims;
  `attested_at` carries the time, which is why `is_safe` takes `max_age_secs`
  against it.
- **Fields in the preimage are escaped.** A claim embeds third-party text, so an
  issuer publishing a directory name containing a tab could otherwise forge the
  preimage of a report that was never produced. There is a test for it.
- **The example gate declares the registry interface rather than depending on
  Assay.** That is what a third-party integrator does, so the example should do
  it too. Its *tests* depend on the registry crate, so an ABI change breaks the
  published example rather than only the registry's own tests.
- **The gate takes the registry address in its constructor.** The same wasm then
  works on both networks; the deployed ID lives in the deploy command and the
  docs, not baked into the binary.

## Verified live, not asserted

Before any attestation existed, and again afterwards against a control asset
that is deliberately never attested:

- `get_safety(unattested)` → `null`
- `is_safe(unattested, max_severity=4, max_age_secs=0)` → `false` — the most
  permissive arguments the gate accepts, still refused
- Through the deployed gate: `deposit` of an unattested asset reverts
  `Error(Contract, #1)`; of clawback-capable `USDZ`, `Error(Contract, #3)`;
  `AQUA` succeeds and credits.

## Known gaps

- **Four assets.** Everything else returns `None`. Correct, but not yet useful
  as a general lookup.
- **One key writes everything.** No multisig, no threshold of independent
  attesters.
- **Nothing re-attests.** Issuer flags can change after a write; freshness is
  entirely the caller's problem via `max_age_secs`.
- **No TTL extension.** Soroban persistent entries expire and nothing bumps
  these. Testnet resets will take them too.
- **`USDZ` is attested but not in the eval set.** The clawback-legitimacy gap is
  now demonstrable but still not *measured* — adding it to
  `internal/mechanics/testdata` is the next step for [eval.md](../eval.md).
- **Horizon only; no RPC path.** Unchanged from the previous session.
