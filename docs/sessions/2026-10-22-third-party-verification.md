# Verification Guide Fresh-Environment Review

Date: 2026-10-22

## Session

Reviewer: GitHub Copilot in the repository workspace. This was a fresh-start
walkthrough using the available workspace environment, not the independent
human session requested by issue #455.

Environment: Ubuntu 24.04.5. `gh` and `sha256sum` were installed; `gh` was
authenticated. Stellar CLI was not installed. The walkthrough started with the
guide's release check. The exact documented download command for `v0.1.0` was
then attempted in a new temporary directory; it exited 1 with `release not
found`. The download command's measured runtime was 0 seconds. No release assets
were downloaded, so the remaining verification steps could not be run. The
overall walkthrough duration and full end-to-end time were not measured; the
procedure was blocked at its first prerequisite.

## Friction and disposition

| Friction | Disposition |
| --- | --- |
| The guide did not state its required tools or the Stellar CLI version. A fresh user could reach the contract-read step without knowing the CLI version used by the project. | Fixed in `docs/verifying.md`: prerequisites and the required Stellar CLI version are now stated before the procedure. |
| The guide used `./assay` after `cd assay-check` without clearly stating that later commands depend on that shell's working directory. | Fixed in `docs/verifying.md`: the shell-state assumption and fresh-shell recovery are now explicit. |
| No public release was available, so the required release download and binary provenance check could not be performed. | Not fixable in documentation. The release prerequisite is now an up-front stop condition; the missing release is already tracked by #278. |
| An independent human verifier was not available in this environment. | Still required: have someone outside the project follow the updated guide and append their elapsed time, environment, failures, and remaining confusion here. This automated walkthrough is not represented as satisfying that criterion. |

## Result

The procedure was blocked at step 0 because no release exists to download. Steps
0b–6, including the Stellar CLI interactions and evidence comparison, remain
unverified by this session.