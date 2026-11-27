# Session: severity and accountability remain separate

**2026-10-05 and 2026-10-23.** The October work made the model's separation
testable at the type and report level, then made the accountability result more
precise without letting it affect severity.

## The boundary is enforced, not just documented

[PR #174](https://github.com/use-assay/Assay/pull/174) added structural tests
that reject paths where accountability could alter severity. This matters
because a verified domain is attribution evidence, not a safety capability:
the issuer's ledger flags continue to determine capability severity, while
reputation may only escalate it.

The follow-up [PR #529](https://github.com/use-assay/Assay/pull/529) split
`accountability: unverified` into `fetch_failed` and `not_claimed` reasons. A
domain fetch outage and a readable TOML that does not claim the asset are
different observations; both remain unverified, neither changes severity.

## What this preserves

The project reports who claims an asset separately from what its issuer can do
to holders. Neither successful attribution nor a failed fetch is a severity
discount or escalation. The evaluation and evidence layers preserve that
distinction for reviewers rather than folding it into a single score.
