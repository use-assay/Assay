# seq8 — capability added, then removed

**Synthetic: true.** Constructed, not captured. The asset is the eval-set
subject whose shape this sequence borrows; it is not a record of that asset's
history. It pins the case issue #599 names: an issuer that holds no capability
enables clawback and then clears it again between three scans — clear,
clawback, clear.

`temporal.Of` compares only the last pair and so sees clawback *removed*, with
no hint that it was ever the end state of a no-change sequence. `temporal.All`
returns both adjacent transitions (added, then removed), and `temporal.Summarize`
names `clawback_enabled` as held at some point and withdrawn by the end. Under
CAP-0035 a trustline opened during the middle window keeps
`is_clawback_enabled` after the issuer clears the account flag, so that window
is exposure the final state does not show.

`mechanics` is the raw bitset and the severity fields are the ABI levels — see
[which bits are capabilities](../../../../docs/transitions.md#which-bits-are-capabilities)
and the [severity model](../../../../docs/severity-model.md).

| observation | at (UTC) | source |
| --- | --- | --- |
| `obs1.json` | 2026-10-01T00:00:00Z | synthetic: constructed for this fixture, shape per docs/transitions.md |
| `obs2.json` | 2026-10-08T00:00:00Z | synthetic: constructed for this fixture, shape per docs/transitions.md |
| `obs3.json` | 2026-10-15T00:00:00Z | synthetic: constructed for this fixture, shape per docs/transitions.md |
