# The severity model

This is the part of Assay worth arguing about. Reading a flag is trivial and
deterministic. Deciding what a flag *means* is the whole product.

The vocabulary this page argues with — `clear`, `unevaluated`, `valid`,
`unknown`, `stale`, `undetermined` — is defined, together with the state each is
most often confused with, in [the glossary](glossary.md).

## The problem

`auth_clawback_enabled` lets an issuer confiscate your balance and burn it,
without your signature. It is also the mechanism a regulated issuer uses to meet
legal obligations — sanctions enforcement, court-ordered reversal, recovering
provably stolen funds. The flag is identical in both cases. The ledger cannot
tell you which one you are looking at, because the difference is not on the
ledger.

A scanner that paints every clawback-capable asset red is wrong about every
well-run regulated asset, and a tool that is wrong about the largest assets on
the network gets ignored. A scanner that waves through anything issued by a
recognizable name is worse: it has quietly become a brand-recognition service
wearing a security tool's clothes.

## The resolution: two axes, never merged

Assay reports **severity** and **accountability** as separate values, and
refuses to combine them into a single score.

| | Severity | Accountability |
| --- | --- | --- |
| Question | What *can* the issuer do to my balance? | Who, if anyone, stands behind that power? |
| Source | Issuer authorization flags | Reciprocal SEP-1 domain, curated directory |
| Nature | Consensus-enforced fact | Published claim |
| Effect | Gates on-chain decisions | Informs a human |

The user decides what to do with the combination. Assay does not decide for
them by averaging a fact with an opinion.

## Rule 1: severity is capability-only

Severity is derived from issuer flags and nothing else. Not reputation, not age,
not trustline count, not who the issuer is.

**Two assets whose issuers hold identical power over holders receive identical
severity.** A household-name regulated stablecoin and an anonymous account
created an hour ago, with the same flags, classify the same.

This is not an oversight. It is the property that makes the on-chain gate sound.
A contract reading `severity <= MEDIUM` is relying on a statement about ledger
mechanics. The moment reputation could lower a severity, that contract would be
relying on a third party's opinion, delivered through a data feed, about an
issuer — and it would have no way to tell the two kinds of claim apart.

It also removes the attack. If attribution lowered severity, the cheapest way
past the gate would be to buy attribution: register a domain, publish a
stellar.toml, get listed. Capability-only severity makes that pointless, because
the flags are what is measured and the flags do not care who you are.

The tests enforce this directly: stripping every trace of attribution from a
subject must not change its severity.

## Rule 2: reputation is monotonic upward

A confirmed malicious listing escalates severity to **Critical**, regardless of
flags. Nothing ever lowers severity.

The asymmetry is deliberate, and it is the correct reading of what the evidence
supports:

- **Presence** on a curated malicious list is a positive observation. Someone
  investigated and concluded abuse. That is decisive.
- **Absence** is not an observation at all. Most legitimate assets are absent
  from scam lists. So is every scam nobody has reported yet. Treating absence as
  a safety signal would mean every brand-new trap scores well precisely when it
  is most dangerous.

Every escalation is auditable: the report carries `base_severity` (capability
alone) next to `severity` (after escalation) and a boolean `escalated`, so you
can always see exactly what reputation contributed.

### Escalation never sets a capability bit

Stronger than the level rule, and the invariant the on-chain gate's correctness
rests on: an escalation finding contributes **no capability bit** to the report
bitset. Bits 0-2 (`auth_required`, `auth_revocable`, `auth_clawback_enabled`)
are the ledger's own record of what the issuer can do, and `CapabilityMask`
(names those three bits in `internal/mechanics/severity.go`) is how a consumer
masks them out of the bitset to read that record. Reputation raising a level
must never read as the ledger granting a power.

Today reputation sets `blocklisted` (bit 5) on escalation — a non-capability
bit, which is exactly the point: a CRITICAL asset with no auth flags must mask
to an empty capability set, and it does.

Nothing relies on convention here. `Engine.Run` rejects an escalation finding
that carries any capability bit with an error rather than masking the bits and
carrying on — silently dropping them would hide exactly the bug the guard
exists to catch — and `TestEscalationNeverSetsCapabilityBits` asserts the
invariant across every eval fixture and over hand-built escalated subjects,
with a negative test proving the assertion fires on a deliberately bugged
finding.

### What a blocklist hit is keyed on

The directory's `malicious`/`unsafe` tags are keyed on the **issuer address**:
StellarExpert made the association between an address and its reputation, so a
directory hit needs no further corroboration to escalate.

The malicious-domain blocklist is keyed on a **domain**, and the only domain
Assay has is the issuer's advertised `home_domain` — a free-text field the
account controls. Using it as the escalation key has a false-positive and a
false-negative face, and the decision below was made with both in view.

| State | What Assay sees | What it does |
| --- | --- | --- |
| verified | blocklist hit on a domain whose `stellar.toml` reciprocally claims this asset | escalates to critical |
| unverified | blocklist hit on a domain that has not claimed this asset | escalates to critical, with the unverified link stated in the finding's reasoning |
| missing | no `home_domain` at all | the lookup cannot be put; reputation is marked `undetermined` and the report names it |

**A hit on an unverified domain still escalates.** A blocklist entry is a
positive observation from a curated source — the same class of evidence as a
`malicious` directory tag — and this model never suppresses positive adverse
evidence. Requiring reciprocal verification before escalating was rejected
because it introduces a silent false negative: the recorded `REPO` scan
escalated *only* through the blocklist, on a domain whose toml did not resolve,
and a verification precondition would have dropped a real scam to `clear`.
Under-reporting risk is a security issue in this project; over-escalating a
legitimate issuer is not the mirror image, because an issuer whose advertised
domain is blocklisted is itself part of the finding. What the report adds is
honesty about the link: the escalation is stated *with the caveat that the
domain has not reciprocally claimed the asset*, so a reader is never handed an
unverified association presented as a confirmed one.

**No `home_domain` is a gap, not a clean result.** With no domain there is
nothing to key the lookup on, so the blocklist is never read. A hit would
escalate, which makes the reported severity a floor. The project's rule that
"we could not check" and "this is fine" must never render the same therefore
applies, and the reputation finding is marked `undetermined` — which also means
such a report is not attestable (`attest.FromReport` refuses it, exactly as it
does for an unreachable source). This is what closes the evasion the issue
raises: an issuer that clears `home_domain` to dodge the blocklist no longer
receives a clean, attestable verdict. The consequence is deliberate and stated:
an issuer that advertises no `home_domain` cannot be checked against the domain
blocklist, and therefore cannot be attested until the lookup can be keyed on
something else. Keying it on the directory's curated domain is the fix, and it
belongs to [#4](https://github.com/use-assay/Assay/issues/4) (directory domain
versus advertised domain) rather than to this decision.

## Rule 3: accountability is reported, never discounted

Reciprocal SEP-1 verification produces `verified`, `unverified`, or `unknown`.
It never moves severity.

Verifying who an issuer is does not reduce what they can do to your balance. It
tells you there is a named party to sue, complain about, or subpoena — which is
genuinely valuable, and is exactly why it gets its own field instead of being
dissolved into a number.

Live data makes the case better than argument does. Circle's USDC issuer
advertises `home_domain = circle.com`, and `circle.com/.well-known/stellar.toml`
returns **404** — on 2026-09-17 it answers with a 301 to `www.circle.com`, which
then returns 404. By SEP-1's own definition, the single most reputable regulated
stablecoin on Stellar failed reciprocal domain verification every time it was
checked (2026-08-10 through 2026-09-17). Had
accountability been wired into severity, USDC would have scored *worse* than a
scam asset that took ten minutes to publish a valid toml.

Assay reports this honestly — `accountability: unverified` — and does not
special-case it.

## Rule 4: do not double-count what the protocol requires

CAP-0035 makes `auth_revocable` a **precondition** for `auth_clawback_enabled`.
Setting clawback without revocable fails with
`SET_OPTIONS_AUTH_REVOCABLE_REQUIRED`, and revocable cannot be cleared while
clawback is set.

So "revocable AND clawback" is not a suspicious combination — it is the only
legal way to have clawback at all. Scoring the two as independent signals would
inflate every clawback asset for a reason the protocol mandates.

Severity is therefore the **highest single capability present**, not a sum.
Confirmed empirically: across 2,400 sampled live assets, exactly zero have
clawback without revocable.

## The SAC admin is evidence, not a severity input

A classic asset's Stellar Asset Contract has an **admin**. It defaults to the
issuer when the SAC is first enabled, and can be moved with the SAC's
`set_admin`, which requires authorization from the current admin — the issuer,
the first time. A SAC admin can be a Stellar account (`G…`) or another smart
contract (`C…`), and the admin's authority is independent of the issuer
account's own signers and thresholds ([CAP-46-06](https://github.com/stellar/stellar-protocol/blob/master/core/cap-46-06.md)).

That matters because the issuer flags Assay reads describe the issuer account,
not the SAC admin. If the admin has moved, the flags no longer describe *who*
can exercise the contract's administrative powers. Clawback through the SAC
still requires the classic asset to have `auth_clawback_enabled`, and
`set_authorized` still requires `auth_revocable` (CAP-46-06), but a locked
issuer can leave a moved SAC admin still able to mint, claw back or freeze. A
contract admin's own logic decides when it does so, and Assay cannot inspect it.

**Decision: the SAC admin is evidence only.** Severity stays capability-only
from the issuer flags, and the SAC admin never raises or lowers it. Two reasons.
First, the power's existence is still the flag's: a moved admin on an asset
without `auth_clawback_enabled` cannot claw back through the SAC. Second,
letting an unverified contract address change a severity would make the number
depend on data the scanner cannot check, which is the same failure mode as
treating attribution as a capability input.

The capability finding states the moved admin, names it, and cites the contract
read — but only when it has moved off the issuer. When the admin still is the
issuer, neither the reasoning nor the evidence changes, so the `evidence_hash`
of an attestation does not move for an asset whose admin never changed. When
the admin could not be read (no RPC endpoint, an RPC failure, an unreadable
entry), the report does not claim it is the issuer: "not read" and "still the
issuer" are different facts, and the finding keeps the latter unsaid.

The behaviour was verified against testnet on 2026-11-23. The issuer-admin case
is `ASSAYPRB-GAIFQB42…`, whose SAC
`CAQRN7WMPPS2VBQQVIUQXGRVYIBXPLT65KIQAI52FJCFFZQ6SG4PP7R4` was deployed by its
issuer in transaction
`ce28eb7dbad21005faff17f39288479c6602d4c3e88f5f385ed1facfb7cbc5e1` and whose
`admin` read back as the issuer. The moved-admin case is
`zUSDC-GAA3S6XLOK…`, SAC
`CAIUOHVZ77RSCDBNWR3BCZPTWHPUXQRTQXSW4VE3HGC2M5PRPJNSFBRU`, whose admin is the
contract `CDFQ5K2BPKB7BWNW2SJPGIIK5OOFQIR434MOX5YYBDKAN3M5CFVJKHR2`. Both raw
ledger entries are committed as fixtures under `internal/sacadmin/testdata/`.

## The levels

Each level is pinned to a specific capability, so the number is
self-documenting on-chain.

| | Level | Flag | What the issuer can do |
| --- | --- | --- | --- |
| 0 | `clear` | *(none)* | Nothing. No special power over holders. |
| 1 | `low` | `auth_required` | Decide who may open a trustline. Cannot touch existing holders. |
| 2 | `medium` | `auth_revocable` | Freeze an existing holder's balance in place. |
| 3 | `high` | `auth_clawback_enabled` | Confiscate and burn a balance, without the holder's signature. |
| 4 | `critical` | *(reputation)* | Reserved for escalation. Never produced by reading flags. |

## What unevaluated means

There is a fifth value on the Go side, `unevaluated`, and it is deliberately
not a level.

**`clear` means the issuer holds no powers — and that is a fact someone
established by reading the flags.** The ledger was asked, the answer came back,
and it said: no `auth_required`, no `auth_revocable`, no `auth_clawback_enabled`.

**`unevaluated` means the flags were never read at all.** No question was
asked, so no answer exists — including the answer "no powers". Collapsing the
two would put the ABI's safest value on a subject nobody assessed, which is
exactly the failure shape behind the bugs already found and fixed: a missing
answer silently rendering as the most permissive one.

In practice the live scanner cannot produce this state — it aborts the whole
scan when Horizon cannot find the asset (`internal/scan/scan.go`), so a
`Subject` with no flags on it only exists when one is built by hand. The value
exists anyway, at the type level, so that nothing can accidentally attest an
unread-flag report: such a finding carries `undetermined`, and
`attest.FromReport` refuses it with `ErrUnevaluated`.

`unevaluated` is not part of the on-chain ABI. It never serializes into an
attestation (see [the contract interface](contract-interface.md)), and the
contract's own range check would reject it as a second backstop. It also never
appears in eval expectations: an eval subject always has flags to read.

## What stale means

A verdict has three potential lifecycle states: **valid**, **unknown**, and **stale**.

- **`valid`** — A fresh, complete verdict. All required sources answered, flags were evaluated, and the observation is within the freshness policy window.
- **`unknown`** — A check could not conclude. Either a required source was unreachable (`undetermined`) or flags were never read (`unevaluated`).
- **`stale`** — The verdict was complete when made, but is now older than the policy window.

Assay learned this distinction through three related failure modes:
1. An unreachable source once rendered as a clean result (#23).
2. Clear doubled as not-evaluated (#32).
3. Stale was unrepresented off-chain, risking an expired verdict being consumed as currently safe (#57).

On-chain, the Soroban example gate already distinguished staleness via `Error::AttestationStale` (`#2`). Off-chain, `Report` now represents `state` (`"valid"`, `"unknown"`, `"stale"`) and `stale` (`true`/`false`) distinctly in JSON. A program reading the report can never confuse an expired clean report with a fresh one. Furthermore, `attest.FromReport` refuses any stale report (`ErrStale`), guaranteeing that an expired verdict cannot be attested as fresh.

`auth_immutable` is deliberately **not** a level. It is not a power over
holders; it fixes whether the power set can change. Which direction that cuts
depends entirely on what is already set:

- Locked with no dangerous flags — a genuine safety property. Clawback can never
  be added.
- Locked with clawback — permanent. The power can never be given up.
- Locked by signers — the same durability without `auth_immutable`: master key
  weight 0, no other signer, and a non-zero medium threshold mean no operation
  can ever be authorized to change the flags.

Assay reports this as its own finding — [`mutability`](checks.md#mutability) —
rather than scoring it, because a single number cannot carry a conditional.

## What severity does not tell you

**Whether the issuer will use the power.** Assay measures capability, not
intent. Nothing on the ledger reveals intent.

**Whether an existing holder is exposed.** Under CAP-0035 the trustline flag
`TRUSTLINE_CLAWBACK_ENABLED_FLAG` is set *when the trustline is created*.
Enabling clawback later does not reach trustlines that already exist.

Assay reports risk for a **prospective** holder — "what happens if I open a
trustline now" — which is what current issuer flags answer exactly. It is not a
claim about anyone's existing position. Checking an existing trustline's own
flags is [a separate check that does not exist yet](checks.md).

**That clawback stops at trustlines.** It does not: CAP-0035 also gives an
issuer whose asset has `auth_clawback_enabled` the `ClawbackClaimableBalanceOp`
operation, which claws back a claimable balance of that asset
([CAP-0035](https://github.com/stellar/stellar-protocol/blob/master/core/cap-0035.md)).
So "can the issuer take my tokens" covers tokens in transit as well as tokens
held on a trustline. That matters to a wallet that uses claimable balances for
onboarding or escrow: the balance can be reclaimed before the recipient claims
it, and the recipient never had a trustline for the issuer to reach. This is a
statement about the reach of a capability Assay already reports, not a new
capability and not a new severity input — an asset's severity is unchanged by
this paragraph. Severity is still derived from the issuer's authorization
flags and nothing else.

**That a `clear` asset is safe.** It means the *issuer* has no special power.
The asset can still be worthless, a rug pull by another mechanism, or an
outright impersonation. `DOGE-GA22IDJN…` in the eval set carries no auth flags
at all and is a known scam.

**Whether the issuer can mint more supply.** An issuer account that is not
locked can issue unlimited new supply. That is dilution risk, not confiscation:
it takes nothing from a specific balance without a signature, and it is not a
capability flag at all — it is the absence of an account lock. It is reported as
context evidence and never moves severity; the decision and its reasoning are in
[issuer mint capability](#issuer-mint-capability-is-context-never-severity).

Assay scans one specific attack surface. It says so rather than implying
coverage it does not have.

## Issuer mint capability is context, never severity

**The question.** An issuer account that has not been locked can issue more of
its asset at any time. Holders therefore face dilution risk: supply the issuer
alone authorizes can reduce the value of every existing balance. Readers ask
about it, and it is not covered by the severity model, whose scope is what the
issuer can do *to a balance* — take it or freeze it.

**Decision: context evidence, never severity.** Of the three options the issue
named — out of scope, attributed context evidence, a separate axis — the answer
is attributed context evidence, reported next to the accountability axis. It
never changes severity, in either direction, and it never sets a capability bit.

Three properties force that answer:

- **Minting is not a flag.** Severity is derived from issuer authorization flags
  and nothing else (Rule 1). Mint capability is the *absence of an account
  lock* — master key weight, thresholds, and signers on the issuer account. It
  lives on a different ledger object from the asset, and a report that folded
  it into severity would gate on-chain decisions on an account's signer layout
  while the report claims to describe the asset's authorization flags.
- **Dilution is not a per-holding power.** Clawback and freeze act on one
  balance, without its holder's consent, and that is precisely what a contract
  reading `severity <= MEDIUM` is protecting against. Mint dilutes all holders
  proportionally and takes nothing from a specific balance. Both are real risks;
  they are not the same question, and answering them with one number would make
  the number mean two things. It would also inflate every credit asset: before
  a lock, *every* issuer can mint, so the level would carry no information.
- **Locking is a conditional, not a capability.** A locked issuer is a genuine
  safety property, and the same argument as `auth_immutable` applies: what it
  means depends on what is already set, and it constrains the future rather than
  the present. It must never lower a severity either, or the cheapest way past
  a gate would be an issuer action that changes nothing about existing holders.

**What the report will carry.** When the issuer-lock check lands, its result is
a finding like any other: attributed to the issuer account record, stating
whether the account is locked and therefore whether new supply can be minted.
It sits beside `accountability` — the other published-claim axis — and is not
averaged, summed, or escalated into the level. An unread lock is `undetermined`
and named as a gap, never rendered as "cannot mint": the rule that "we could
not check" must not look like "this is fine" applies to this question exactly
as it does to the domain blocklist.

**Why not a separate axis yet.** A third axis is a compatibility commitment:
its value would enter the report schema and, once attestations carry it, the
evidence preimage. No consumer asks for that yet, so the smallest honest form is
context evidence. Recording the decision now means a future mint-risk axis
starts from an argument rather than a redesign — and, until then, nothing about
the severity contract changes. The invariant that reputation must never set a
capability bit applies here unchanged: a mint finding contributes no severity
and no capability bit, and the tests that strip attribution from a subject must
continue to leave severity byte-identical.
- [Temporal Trust Model](temporal.md)
