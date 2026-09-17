# Session: audit and provenance

**2026-09-17.** An integrity audit, not feature work. It checked what is merged,
what the tests actually catch, whether the evidence hash matches its
documentation, whether the merge gate holds what it claims to, and how the
repository's history was produced.

## Tool provenance

This repository's commits were written with AI coding tools. That is recorded
here because it was previously removed from commit messages, and an appeal or a
reviewer should not have to infer it.

**Codebuff.** Eight commits merged in
[PR #27](https://github.com/use-assay/Assay/pull/27) were produced in a Codebuff
session on 2026-09-16. Each originally ended with the trailers
`🤖 Generated with Codebuff` and `Co-Authored-By: Codebuff <noreply@codebuff.com>`.

| On `main` | Original commit (with trailers) | Committed | Subject |
| --- | --- | --- | --- |
| `5aa88e9` | `e1f6c7c` | 2026-09-16 01:49 +0100 | Fix the example gate to refuse critical-by-reputation assets |
| `8b6feca` | `ef0eb1d` | 2026-09-16 01:49 +0100 | Test that attest rejects a caller that is not the admin |
| `e76acf8` | `f19af80` | 2026-09-16 01:54 +0100 | Pin the evidence_hash preimage to the documented encoding, byte for byte |
| `64bbfc2` | `e63859e` | 2026-09-16 02:21 +0100 | Widen the attestation run to 18 assets; record where the plan was wrong |
| `b64b93c` | `ae6355c` | 2026-09-16 02:26 +0100 | Make the merge gate a script that can be verified offline |
| `8d7cecd` | `21bd71c` | 2026-09-16 02:32 +0100 | Drop non-standard front matter from the PR template |
| `797544e` | `9da7194` | 2026-09-16 02:33 +0100 | Record the third tranche's three attestations in the deployment ledger |
| `3c928e1` | `b026438` | 2026-09-16 08:03 +0100 | Redeploy the example gate with the severity ceiling and record it |

The trailers were removed before anything was pushed, by a Claude Code session,
on the repository owner's instruction at the time (a standing rule of "no AI
attribution trailers"). Only the trailer lines changed. Each commit's tree,
author, and author and committer dates were verified identical before and after.
The original commit objects are kept in the maintainer's local clone under
`refs/provenance/`; they were never pushed.

That removal is not repeated or reversed here. Rewriting published history
again, in either direction, would be a second concealment problem. This record
is the correction instead.

What the repository does not establish: how the Codebuff session was started,
where it ran, or why it ran against this branch at the same time as a Claude
Code session. Neither is claimed here. The two sessions overlapped: both
redeployed the fixed example gate six minutes apart, which is why a duplicate
instance exists (see [deployment.md](../deployment.md#example-gate-instances)).

**Claude Code (Anthropic).** The commits made in the 2026-08-15, 2026-09-05,
2026-09-16 and 2026-09-17 sessions — every non-merge commit on `main` from
`e680161` onward other than the eight above — were written by Claude Code working
in this repository under the owner's direction. Merge commits, such as
`412b566` for PR #27, were made by the owner on GitHub.

One mixed case: `d61c001` also contains edits the Codebuff session left
uncommitted in `README.md`, `docs/integrating.md` and `docs/attestation-run.md`.
Most were gate-address updates. Claude Code reviewed them, corrected one claim
that a same-day re-attestation had made false, and committed them together
with its own changes. They carry no attribution trailer, under
the same standing rule. This note does not cover commits before 2026-08-15; the
repository does not record how those were produced.

**Commit clustering.** Several sessions produced commits minutes apart: four on
2026-08-15 within one minute, five on 2026-09-05 within 36 minutes, and seven on
2026-09-16 within 44 minutes. None of it is backdated. The timestamps are the
times the commits were made.

As of this audit, Codebuff has no access to the repository of its own. On
GitHub there is no Codebuff app, deploy key, webhook or collaborator; the only
installed app is `drips-wave` (issues and pull requests, no code access). On the
maintainer's WSL machine there is no Codebuff install, cached package, process
or git hook. It could only have written using the owner's own git and GitHub
credentials, from wherever it ran.

## What the audit checked

- **Contract tests.** 19 pass: 11 registry, 8 example gate. Three mutations
  were applied in a scratch copy and each was caught by its test: removing
  `require_auth` from `attest`, making `is_safe` return `true` for an
  unattested asset, and removing the example gate's severity ceiling.
  `attest_rejects_unauthorized_caller` is weaker than its name suggests: it
  tests an `attest` call with no authorization at all, not one signed by the
  wrong account.
- **Evidence hash.** For all ten on-chain attestations, a re-implementation
  built only from `contract-interface.md`, `assay attestation -raw`, and
  `get_safety` agree byte for byte. Three of those hashes commit to raw
  transport-error text ([#24](https://github.com/use-assay/Assay/issues/24)). See
  [attestation-run.md](../attestation-run.md#reproducibility-audit-2026-09-17).
- **Merge gate.** It let six safety-critical paths through, including the
  severity code and the gate itself, and it listed a file that does not exist.
  Both were fixed, and 19 probe scenarios then behaved as intended. The gate
  checks paths, not the PR checklist.
- **Branch protection.** `main` has no protection rule and no ruleset, so the
  gate reports but does not block. CI on `main` was red from 2026-09-05 until the
  Go toolchain was pinned; the merge of PR #27 is the first green run since.
- **Dated claims.** Every live claim about attestations, freshness, hashes or
  addresses now carries a date and a source.
- **Sample size.** Held at 18. The reasons are in
  [attestation-run.md](../attestation-run.md#what-this-run-does-not-establish).
