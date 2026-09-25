# Checks

A check is a pure function from a pre-fetched `Subject` to a `Finding`. Checks
perform **no I/O**: the [`scan`](../internal/scan) package fetches everything
once, up front, and hands the result to classifiers that only compute.

That split buys three things. Checks are deterministic and testable from
fixtures with no network. Each fetcher stays in its own package with a clean
signature, so it can be extracted later without untangling judgment logic from
transport. And a check cannot quietly add a network dependency, because it has
nowhere to put one.

```go
type Check interface {
    ID() string
    Describe() string
    Run(ctx context.Context, s *Subject) (Finding, error)
}
```

## `capability`

**Concludes:** what the issuer is able to do to a holder's balance.
**Sets:** base severity.

Maps issuer authorization flags to a level, per
[the severity model](severity-model.md). Severity is the highest single
capability present, not a sum — `auth_revocable` is a protocol precondition for
`auth_clawback_enabled`, so counting both would double-count a rule CAP-0035
enforces.

The reasoning always states the raw capability in plain language, whatever the
level works out to. A reader is never told a number without being told what the
issuer can actually do.

`auth_immutable` is not scored here: whether the flag set can still change is a
separate, first-class finding — see [`mutability`](#mutability).

**Cannot conclude:** whether the issuer will ever use the power; whether an
existing holder is exposed (clawback is inherited at trustline creation, so this
answers the prospective question only); or anything about who the issuer is.

## `mutability`

**Concludes:** whether the issuer's authorization flag set can still change.
**Sets:** nothing. Severity stays `Clear` in every case.

`auth_immutable` is not a power over holders, so it is deliberately not a
severity level — see [the severity model](severity-model.md). Its meaning is
conditional, and this check reports the condition rather than folding it into a
number. There are three states, each with its own reasoning:

- **Locked with no dangerous flags** — a durable safety property. Because the
  flag set can never change, `auth_revocable` and `auth_clawback_enabled` can
  never be added, so a `clear` verdict today is also `clear` for a trustline
  opened later.
- **Locked with freeze or clawback** — permanence. No flag in the set can be
  given up, because `AUTH_IMMUTABLE_FLAG` cannot be cleared once set (CAP-0035
  makes all `AUTH_*` flags read-only). Locking does not make the power safer; it
  removes any future in which the issuer relinquishes it.
- **Not locked** — the issuer may add freeze or confiscation later. Under
  CAP-0035 a flag added later does not reach trustlines that already exist, but
  it applies to any trustline opened after the change. This is the state a
  prospective holder wants surfaced, because it is the one where today's `clear`
  is not a durable answer.

**Flag source.** The check reads the issuer flags from the same Horizon
`/assets` record the `capability` check derives severity from, so the two
findings cannot disagree about the flag set. `auth_immutable` is also carried on
the issuer `/accounts` record; that is a second ingestion path Assay does not yet
reconcile, and taking the `/assets` record here keeps this finding consistent
with the severity reported beside it. The gap is recorded in
[the attestation run](attestation-run.md#what-the-checks-can-get-wrong).

**No evidence.** The flag read is already attributed by the `capability`
finding's Horizon evidence. `evidence_hash` is a SHA-256 over a **sorted list**
of evidence lines, so a second copy of the same claim — or a new claim about the
same read — would change the hash of every attestation whose underlying evidence
did not change, which the encoding is explicitly designed to prevent. This check
therefore reasons over the read that is already attributed rather than
re-emitting it.

**Cannot conclude:** what the issuer will do with the power, or whether an
existing holder is exposed. It reports only the durability of the flag set,
which is a fact about the issuer account. It is not a safety verdict on its own:
an unlocked `clear` asset is still only clear about issuer capability, not about
whether the asset is genuine.

## `sep1-domain`

**Concludes:** whether an identifiable party has publicly claimed this asset.
**Sets:** accountability. **Never touches severity.**

Reciprocal verification, requiring both directions to agree:

1. The issuer account advertises `home_domain`.
2. That domain serves `/.well-known/stellar.toml`.
3. The toml's `CURRENCIES` lists **this code and this issuer**.

Either half alone is worthless. `home_domain` is free text any account can set
to any string; a stellar.toml can list any code it likes. Matching on code alone
would let any domain claim any asset — the exact impersonation this check
exists to catch — so both must match.

Verification failures are reported verbatim, including the HTTP status. "We
could not check" and "this is fine" are different answers and must never render
the same.

That rule is repo-wide, not local to this check. It was stated here first
because `stellar.toml` was the only source that honoured it; the two
StellarExpert endpoints did not, and a scan during an outage claimed reputation
had been read when it had not. Fixed, with the history in
[the attestation run](attestation-run.md#finding-1).

The same rule applies to negatives. SEP-0001 permits a currency entry whose only
field is `toml="https://DOMAIN/.well-known/CURRENCY.toml"`, delegating the
declaration to a separate file. Such an entry carries no code or issuer, so it
can never match. Assay does not follow those links yet, so when a toml contains
them it reports the asset as **unconfirmed** rather than claiming the domain
failed to name it — overstating a negative is the same class of error as
overstating a positive. Following those links is not implemented yet.

**Cannot conclude:** that a verified issuer is honest. It establishes that a
named party has published a claim, nothing more. A scammer can register a domain
and publish a valid toml in ten minutes; that is precisely why this sets
accountability rather than severity.

## `reputation`

**Concludes:** nothing of its own. It relays curated third-party determinations.
**Sets:** escalation only, upward only.

Consumes StellarExpert's address directory (the data set standardized by
SEP-0037) and malicious-domain blocklist. A `malicious`/`unsafe` directory tag
or a blocklist hit escalates to `critical`.

Everything it produces is `Evidence{Source, URL, Claim, RetrievedAt}` naming
StellarExpert and the URL the claim came from. Attribution is structural: a
check can only surface an outside claim by constructing an `Evidence`, so there
is no code path that renders someone else's data as an Assay conclusion.

Assay does not maintain a scam list, a rating, or a domain blocklist. That layer
exists, is actively curated, and is better than anything this project would
produce. **Do not re-derive it.**

**Cannot conclude:** that an unlisted asset is safe. Absence from a curated list
is not an observation — most legitimate assets are absent, and so is every scam
nobody has reported yet. The check says so explicitly in its reasoning rather
than staying silent and letting absence read as approval.

**Cannot conclude anything at all when a source did not answer.** A 404 means
the source was read and had no entry; a 429, a 5xx, or a timeout means it was
never read. Those produce different reports: an unreachable source is recorded
as attributed evidence carrying the failure verbatim, the finding is marked
`undetermined`, and the report names the check on `undetermined_checks`.

Severity is **not** raised to compensate. Capability stays exactly what the
ledger says, because inventing a level Assay did not measure would be the same
error pointed the other way. What changes is that the report states the level is
a floor, and that `attest.FromReport` refuses to write it on-chain at all —
`get_safety` returns a severity and a timestamp with nowhere to carry the
caveat, so a partial scan must not become an attestation. The contract already
handles the resulting absence correctly: `None`, and every gate fails closed.

A listing that *did* arrive still escalates even if the other source is down.
Evidence of abuse does not become less true because a second endpoint timed out,
and `critical` is the ceiling, so nothing still missing could raise the level
further.

Note that the two sources are not interchangeable and neither is complete. When
`BERKSHIRE-GA22QHSH…` was scanned, the directory tagged the issuer `malicious`
while blocked-domains returned `blocked=false` for the same domain. Consulting
only one of them would have missed a confirmed scam.

## Capability transitions

The checks above answer a prospective question: what can this issuer do to a
trustline opened now. Comparing two of those answers over time is a separate,
pure computation in [`internal/temporal`](../internal/temporal): what capability
bits were added or removed between two observations, and which axis — capability
or reputation — moved severity. It performs no I/O and stores nothing. The
representation and the on-demand-versus-stored decision are in
[transitions.md](transitions.md).

## Adding a check

See [CONTRIBUTING.md](../CONTRIBUTING.md). The short version: implement the
interface, verify every flag name and endpoint against a live source, and add
eval subjects for both a true positive **and** a legitimate use of the same
mechanics. A check whose judgment is not evaluated does not ship.
