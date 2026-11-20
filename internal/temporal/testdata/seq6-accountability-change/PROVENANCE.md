# seq6 — accountability change

**Synthetic: true.** Constructed, not captured. No committed sequence records a
domain losing its reciprocal SEP-1 claim between two scans, so this pair is
built to the shape [docs/transitions.md](../../../../docs/transitions.md)
defines: capability bits and severity stay put while `accountability` moves
`verified` → `unverified`. That is the case the evidence detectors cannot see,
because accountability is not an evidence entry.

| observation | at (UTC) | source |
| --- | --- | --- |
| `obs1.json` | 2026-10-01T00:00:00Z | synthetic: constructed for this fixture, shape per docs/transitions.md |
| `obs2.json` | 2026-10-08T00:00:00Z | synthetic: constructed for this fixture, shape per docs/transitions.md |
