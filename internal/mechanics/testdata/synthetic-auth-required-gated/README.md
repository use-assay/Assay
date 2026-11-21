# synthetic-auth-required-gated

**Synthetic.** Nothing here was captured from a live source, and no real asset is
described. The directory exists to pin the `low` severity level, which no
captured corpus subject exercises: every real fixture Assay has ever captured is
either flag-free, revocable, or clawback-enabled, and `auth_required` alone is
none of those.

## Construction

- `asset.json` is the shape Horizon's `/assets` endpoint returns, with
  `flags.auth_required = true` and the other three flags false. The asset is
  `GATED-GAAAA…WHF`; the issuer is the canonical all-zero ed25519 key, which is
  a well-formed account ID shape used across this repository's tests and is
  nobody's real issuer.
- `account.json` is the matching `/accounts/{issuer}` record, carrying the same
  flags so `reconcileFlags` sees no disagreement, and a `home_domain`
  (`gated.example`) that does not resolve.
- `stellar.toml.status` is `404`, the convention for "the domain was asked and
  did not publish a toml" (`docs/adding-a-check.md`). It makes accountability
  `unverified`, not `verified`, so the fixture cannot accidentally suggest that
  the gating flag came with a claimed identity.
- No `directory.json` or `blocked.json` is present because the synthetic issuer
  is not listed anywhere; the loader treats an absent curated entry as "not
  listed", which is the normal case.

## What it proves

`auth_required` maps to `low` and to nothing else: the issuer can decide who may
open a trustline, and cannot touch a balance. The test that pins it is
`TestAuthRequiredIsLow` in `internal/mechanics/check_capability_test.go`.

This directory is deliberately **not** in the labelled eval corpus
(`internal/eval/corpus.go`): it has no `metadata.json`, so it is a unit-test
fixture rather than a judged subject, and adding a synthetic asset to a precision
measurement would weaken what that measurement means.
