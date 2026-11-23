# seq7 — withdrawn listing (de-escalation)

**Synthetic: true.** Constructed, not captured. The asset is the eval-set subject
whose shape this sequence borrows (`DOGE-GA22IDJN…`, already used by
[seq2](../seq2-reputation-escalation/PROVENANCE.md)); neither observation is a
record of that asset's real history. No capture in this repository holds a
listing being withdrawn, which is exactly why the case is constructed and
labelled as such.

The point of the sequence: final severity **falls** because a consumed source
withdrew the claim that escalated it. `base_severity` never moves — no
capability bit is involved, only reputation — and the derived transition is a
typed de-escalation naming `stellar.expert/directory`, the source whose listing
disappeared. See
[issue #388](https://github.com/use-assay/Assay/issues/388) and
[docs/threat-model.md](../../../../docs/threat-model.md#4-reputation-attribution-age-or-popularity-lowers-a-severity),
attack class 4, which this sequence makes observable with a test.

| observation | at (UTC) | source |
| --- | --- | --- |
| `obs1.json` | 2026-09-16T00:00:00Z | synthetic: constructed for this fixture, state while the curated listing stood (tags: malicious) |
| `obs2.json` | 2026-10-02T00:00:00Z | synthetic: constructed for this fixture, state after the listing was withdrawn |
