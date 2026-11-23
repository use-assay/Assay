# synthetic-auth-required-gating

Synthetic fixture for issue #376.

The asset record and account are derived from `aqua-clear-verified` with one
change: `auth_required` is set to `true` in both `asset.json` and
`account.json`. The `stellar.toml`, `blocked.json` and `directory.json` are
copied unchanged from that subject.

**What this exercises:** `auth_required` as a gating mechanism. The issuer
must approve each trustline before a holder can receive the asset. This maps to
`Low` capability severity. The domain check still passes because the AQUA
stellar.toml claims the asset with both code and issuer, so accountability is
`verified`.

**Why synthetic:** No live asset with `auth_required` and clean reputation
was identified during the Wave 11 contribution window. A synthetic fixture is
labelled as such and exercises the same judgment paths a live one would.
