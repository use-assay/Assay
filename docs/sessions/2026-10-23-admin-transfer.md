# Session: make registry administration rotatable

**2026-10-20 through 2026-10-23.** The registry began with a one-time admin
address, which made a lost or intentionally rotated key require redeployment.
The October work changed that into an observable, two-step handover without
changing the attestations already stored.

## Two-step transfer

[PR #497](https://github.com/use-assay/Assay/pull/497) added admin getters,
transfer TTL behavior, and the admin-rotation threat model. The intended
sequence is proposal by the current admin, then acceptance by the proposed
admin; neither a typo nor an unowned address takes control in one call.

[PR #501](https://github.com/use-assay/Assay/pull/501) added
`cancel_admin_transfer`, so the current admin can withdraw a pending proposal.
[PR #502](https://github.com/use-assay/Assay/pull/502) made proposal and
acceptance visible in registry events, and [PR #519](https://github.com/use-assay/Assay/pull/519)
completed the entrypoint test matrix.

The handover changes only the writer role. Existing safety records remain
readable and unchanged; the former admin loses write authority after acceptance,
and the new admin gains it. The live testnet deployment predates these
entrypoints, so the code change does not rotate that deployment's key in place.
