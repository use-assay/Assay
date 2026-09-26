# Security policy

## Reporting a vulnerability

Report privately via [GitHub's security advisory form](https://github.com/use-assay/Assay/security/advisories/new),
or by email to deekhay7534@gmail.com.

Please do not open a public issue for anything in the first category below.

There is no bug bounty. Assay is an early open-source project and cannot
offer payment.

## What counts as a vulnerability here

Assay is not a service holding user data, so the usual list does not map
cleanly. What Assay produces is a **claim about whether someone's money can be
taken**, and a contract that gates on that claim. The security-relevant failures
are the ones that make a claim wrong in the dangerous direction.

### Critical: anything that under-reports risk

These are the bugs that matter most, because someone acts on the output.

- A scan reporting a severity **lower** than the issuer's flags justify.
- An asset whose issuer can confiscate (`auth_clawback_enabled`) classifying
  below `high`, or `ConfiscationMask` failing to imply `high`.
- Any path where attribution, reputation, age, or popularity **lowers** a
  severity. Severity is capability-only by design; a discount is a
  vulnerability, not a feature request. It would also create the obvious attack:
  buy attribution to get past a gate.
- `is_safe` returning `true` for an asset that is unattested, stale, or above
  the caller's threshold — any failure to fail closed.
- A forged or replayed attestation accepted by the registry.
- A parsing bug letting a stellar.toml claim an asset it does not issue, or
  letting a code-only match pass as reciprocal verification.

### Also in scope

- Severity **inflation** that is systematic rather than incidental. A scanner
  that cries wolf gets ignored, and an ignored scanner protects nobody.
- Presenting a consumed third-party signal as an Assay conclusion, or dropping
  the attribution and source URL from evidence.
- Displaying a value that was never fetched, or rendering a failed fetch as a
  clean result. "We could not check" and "this is fine" must never look the
  same.
- Injection through issuer-controlled content — asset codes, `home_domain`,
  stellar.toml fields, directory names — into the API response or the UI.
  Issuer-controlled strings are untrusted input.
- Denial of service in the fetch layer: unbounded reads, missing timeouts, or a
  hostile domain able to stall a scan.

### Out of scope

- **An asset being a scam that Assay rates `clear`.** Assay measures issuer
  trap mechanics, not fraud in general. An asset with no authorization flags
  genuinely gives its issuer no special power, and Assay says so; the asset can
  still be worthless or an impersonation. This is documented in
  [docs/severity-model.md](docs/severity-model.md#what-severity-does-not-tell-you)
  and there is a subject in the eval set for exactly this case.
- Accuracy of third-party data. StellarExpert's directory and blocklist are
  consumed and attributed, not curated by us. Report those upstream.
- An issuer changing its flags after a scan. Reports are point-in-time and carry
  a timestamp; the contract exposes `attested_at` so callers can set their own
  staleness policy.
- Missing checks. A mechanic Assay does not examine yet is a feature request —
  open an issue.

## Supported versions

Pre-1.0. Only `main` is supported; there are no maintained release branches.

## Deployment status

`assay-contracts` is deployed to Stellar testnet. Pubnet is not deployed.

Current addresses live in [docs/deployment.md](docs/deployment.md); this file
does not repeat them so they cannot drift out of sync.

The canonical gate instance is `CAL5VYSWLKG367D5IYGI57XH7EMN5PLJ4CD6K3MO2HJBYYKEKPG3NKRX`.
A superseded gate instance, `CANO57JRGTATHGLM26TWYPIXERSPVI5R52H33K7ZUJGGOEOVVZA44W3U`,
is known-flawed: it admits critical-by-reputation assets (see [#26](../../issues/26)).
Do not treat reports against the superseded instance as new findings against
the canonical one.

Please still report design flaws in the ABI or the gate logic — the value of
finding them is highest before more depends on them.

Known limitations, documented in
[docs/contract-interface.md](docs/contract-interface.md#not-done-yet): a
single admin key can write any attestation. `evidence_hash` has a canonical
encoding, specified in [#11](../../issues/11) and verified byte-for-byte
against the documented procedure on 2026-09-17.