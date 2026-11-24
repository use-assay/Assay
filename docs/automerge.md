# Automerge

A bot (`.github/workflows/automerge.yml`, logic in `.github/scripts/automerge.js`)
merges pull requests that GitHub reports as mergeable, and leaves one comment
on those that are not, saying what to fix.

It squash-merges a PR when:

- GitHub says it is mergeable: no conflicts and every requirement of the `main`
  ruleset met (`Go`, `golangci-lint`, `Soroban contract`, `Merge gate`, `Malware scan`);
- the `Malware scan` status passed and the PR has no `security-hold` label;
- no reviewer has an unresolved "changes requested";

The bot adds no rules about which paths a PR touches. The `Merge gate` check still
runs and flags maintainer-owned paths for reviewers, but it is advisory.

It also starts CI for first-time contributors, but only after the malware scan
passed on that commit, and never for PRs that change `.github/` or `scripts/`.

Add the `no-automerge` label to keep a PR out of it. To try a change to the bot
without side effects, run the workflow manually with `dry_run` on.
