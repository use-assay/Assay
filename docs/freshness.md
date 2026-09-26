# Attestation freshness and re-attestation policy

**Status: policy document.** It answers
[#9](https://github.com/use-assay/Assay/issues/9): when an attestation must be
refreshed, how a consumer should choose `max_age_secs`, and — stated honestly —
how much exposure the gaps between attestations actually carry.

The contract deliberately leaves freshness to the caller: `is_safe` takes
`max_age_secs` and fails closed on a stale attestation
([contract-interface.md](contract-interface.md#staleness-is-the-callers-policy)).
That is the right split for the *mechanism* — a DEX listing gate and a large
settlement have very different tolerances — but a mechanism without a policy
is an unfilled-in form. This document fills it in, with reasoning a consumer
can disagree with explicitly rather than silently.

Read alongside [attestation-writer.md](attestation-writer.md), which designs
the thing that actually performs re-attestation; nothing here is enforceable
until that exists.

## What attestation freshness is *for*

An attestation is a claim that a scan at time T found the issuer holding
capability C. It goes stale in two independent ways:

1. **The claim ages.** The world may have changed since T.
2. **The claim erodes.** Even unchanged, an old attestation was verified by
   fewer contemporaneous observers, and the pipeline that would refresh it may
   have silently stopped (the failure mode
   [attestation-writer.md](attestation-writer.md#5-failure-visibility) exists
   for).

`max_age_secs` covers both at once, which is why it is a single knob and why
no recommendation here pretends to distinguish them.

## Recommended `max_age_secs` bands

These are bands with reasoning, not a magic number. The reasoning is the point:
a consumer that needs a different number should be able to say which of these
arguments does not apply to it.

| Band | `max_age_secs` | For consumers whose worst case is… | Reasoning |
| --- | --- | --- | --- |
| **Settlement-grade** | ≤ 3600 (1 hour) | irrecoverable loss on a single action | A settlement is a one-shot, high-value action; the marginal cost of requiring a fresh attestation is one more pipeline cycle, and the exposure window (below) is only ever as long as the pipeline's scan interval. Anything looser makes the gate's per-call guarantee weaker than the action it protects. |
| **Market-infrastructure** | ≤ 21600 (6 hours) | many users routed by one automated gate | A DEX listing gate or an aggregator checks constantly but per-asset risk accrues slowly; the binding constraint is that *some* fresh verification exists within the same trading day. 6h keeps worst-case exposure comfortably under a day even if the event-driven trigger (below) misses. |
| **Monitoring-grade** | ≤ 86400 (24h) | dashboards, notification, research | The consumer acts on human timescales; a day-fresh claim is materially the same as an hour-fresh one for deciding what to look at. This is also the example gate's window, kept as the upper bound of *automated* acceptance. |
| **Never acceptable** | > 604800 (7 days) | — | An attestation older than a week is not a freshness policy, it is an archaeological record. A consumer wanting this is gating on nothing and should read `get_safety` without `is_safe` and say so honestly. |

Two rules that hold across all bands:

- **The band is a ceiling, not a target.** `max_age_secs = 0` (freshness
  disabled) is legitimate only for reads that are explicitly not gates —
  displaying what the registry says, or the verification procedure in
  [deployment.md](deployment.md), which deliberately needs no freshness at all.
- **Tighter is always safe.** Nothing in the registry or the pipeline breaks
  if a consumer demands fresher attestations than the schedule delivers; it
  just fails closed more often. That asymmetry — cost of tightness is
  availability, cost of looseness is safety — is why the bands lean strict.

## The re-attestation cadence

**Decision: daily scheduled re-attestation of the whole attested set, plus
event-driven re-attestation of an asset as soon as a scan observes its
capability change.**

- **Daily floor.** Every attested asset is re-scanned and re-attested at least
  once every 24 hours, so the *monitoring-grade* band above is always
  satisfiable. Re-attestation with identical evidence moves only `attested_at`
  (demonstrated live on AQUA and DOGE,
  [deployment.md](deployment.md#the-26-fix-before-and-after)), so a daily
  refresh is cheap in both fees and hash churn: fees are negligible
  ([attestation-writer.md](attestation-writer.md#4-which-assets-and-who-pays)),
  and `evidence_hash` does not move unless the evidence did.
- **Event-driven trigger.** The writer's watcher re-scans attested assets'
  issuers on a short interval (scan cost permitting; the scans are three HTTP
  calls). On a capability addition or severity escalation — the transitions
  [`temporal`](../internal/temporal) detects — the asset is re-attested
  immediately rather than waiting for the daily slot. The trigger is
  detection-driven, so its latency is one scan interval, not one day.
- **No re-attestation on reputation-only movement is required** for
  freshness, because reputation lives in the same `severity` field and a
  re-scan re-derives it; the event trigger fires on it the same way. The
  daily floor covers both axes regardless of trigger misses.

The cadence is a property of the *writer*; the bands above are a property of
the *consumer*. They meet in the middle: the daily floor exists so the 24h
band is honest, and the event trigger exists so capability *additions* do not
wait for it.

## The exposure window, stated honestly

**An issuer can enable `auth_clawback_enabled` at any time, and every
trustline opened after that change is exposed. The window between the change
and the next attestation is real risk surface, and no cadence closes it.**
Stated in pieces, because the honest answer is not one number:

- **Chain-visible immediately.** Flag changes are ordinary `SetOptions`
  operations; they take effect at ledger close and are visible in Horizon the
  moment that close lands (~5 s cadence, confirmed live 2026-09-25 on both
  pubnet and testnet). Horizon does not lag the change in any way a scanner
  could not see with a fresh fetch — this was explicitly verified before
  writing this policy rather than assumed.
- **Scanner-visible within one scan interval.** The event-driven watcher
  re-scans on a short interval; a flag change is detected on the first pass
  after it lands. Detection latency is therefore minutes under normal
  operation — but it is *bounded by the interval*, not zero, and a watcher
  outage stretches it to the daily floor.
- **Attestation-visible within one write cycle.** Detection then submission;
  the write lands a ledger or two later.
- **Consumer-visible only at the next gated call.** This is the part no
  pipeline fixes: between the flag change and the next `is_safe` read of a
  *refreshed* attestation, a consumer using band N admits based on the old
  attestation. With the event trigger healthy, that window is minutes; with
  the watcher down, up to the consumer's `max_age_secs`.

**The CAP-0035 caveat that materially changes the urgency, re-read from the
spec before writing this:**
[`cap-0035.md`](https://github.com/stellar/stellar-protocol/blob/master/core/cap-0035.md)
requires that "`AUTH_CLAWBACK_ENABLED_FLAG` … must be set when a trustline is
created to authorize a `ClawbackOp`". Clawback power is **inherited at
trustline creation**: a holder whose trustline predates the flag is not
exposed to `ClawbackOp` on that balance, and `SetTrustLineFlagsOp` cannot add
`TRUSTLINE_CLAWBACK_ENABLED_FLAG` retroactively. So:

- For **existing holders**, a newly-enabled clawback flag changes nothing
  about their current balance. The exposure window above applies to them at
  **freeze** severity (`auth_revocable` *does* reach existing trustlines —
  freezing is an issuer-side `SetTrustLineFlagsOp` away, no inheritance
  required), not at confiscation severity.
- For **new trustlines opened inside the window**, exposure is full: clawback
  attaches at creation. The window is most dangerous precisely for the
  people the gate is trying to protect — those deciding whether to open a
  trustline *now* — which is why the event trigger exists and why
  `assay scan` remains available to anyone who wants a point-in-time answer
  rather than a possibly-stale attested one.

This is also why the severity model's prospective framing
([severity-model.md](severity-model.md)) is the right one for the scan and
why an attestation's `attested_at` is the right thing for a gate to lean on:
the scan answers "what can the issuer do to a trustline opened now", and
`max_age_secs` bounds how far from *now* that answer is allowed to sit.

## What a consumer should do on `false`

`is_safe` returning `false` is fail-closed, and it collapses four different
worlds. A consumer should distinguish them before deciding what `false` *costs*,
because the responses differ:

| Why it was false | How to tell | Reasonable response |
| --- | --- | --- |
| **Never attested** | `get_safety` → `None` | **Block** for settlement-grade gates; **degrade** (scan on demand, or warn with the unattested status shown) is defensible where blocking every unattested asset would make the product useless. Never silently pass. |
| **Stale** | `Safety.attested_at` older than the consumer's `max_age_secs` | **Degrade or retry**: the claim may be about to refresh. A short backoff-and-recheck (seconds to minutes) is often right; a settlement should still block rather than proceed on the stale value. |
| **Too severe** | `severity > max_severity` or a refused bit in `flags` | **Block.** This is the gate working as designed; the issuer genuinely holds power the consumer has declined. Do not "degrade" a refusal into a warning — the whole model is that capability decides. |
| **Internally inconsistent** | clawback bit set below `SEVERITY_HIGH` (contract-checked) | **Block and report.** This is a writer bug or tampering;
  [report it as a security issue](../SECURITY.md), not an operational one. |

The general shape: **block when the answer is "no" about capability, degrade
when the answer is "we don't know yet", and never let a degrade path render
the same output as a pass.** That last clause is the repo-wide rule —
"'we could not check' and 'this is fine' are different answers"
([checks.md](checks.md#sep1-domain)) — applied at the consumer boundary,
where it matters most.

## Invariants this policy must never break

- **No change that lets a stale attestation pass.** Every recommendation here
  tightens or leaves alone the fail-closed behaviour in
  [`is_safe`](../assay-contracts/contracts/safety-registry/src/lib.rs); the
  existing `stale_attestation_fails_closed` test is the line in the sand.
- **Freshness never changes severity.** An old attestation of `high` and a
  fresh attestation of `high` carry the same capability claim; age is a
  separate axis the consumer owns. Nothing here introduces a time-based
  severity adjustment, which would smuggle a reputation-like input into the
  capability gate.
