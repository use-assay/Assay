# Attestation run

Every asset Assay has attested on-chain, what each check returned, and what the
run exposed about the scanner itself.

This document exists to be checked rather than believed. Every figure below came
from a live command or an on-chain record, and each section says which command
produces it. Nothing here is reconstructed from an earlier write-up.

- **Registry** `CBK4FBIHMDTXCUPE4E3ZDVSFJSCY5FJETTKNIQPN4LFJIKKIBLKIXQ73` (testnet)
- **Example gate** `CANO57JRGTATHGLM26TWYPIXERSPVI5R52H33K7ZUJGGOEOVVZA44W3U` (testnet)
- **Attested** 2026-08-15 · **Re-verified** 2026-09-05

To reproduce any row:

```sh
./assay scan CODE-ISSUER              # the findings and reasoning
./assay attestation -raw CODE-ISSUER  # severity, flags, evidence_hash
./assay attestation -preimage CODE-ISSUER   # the exact bytes hashed
make read ASSET=CODE-ISSUER           # what the contract actually stores
```

## Sample so far

Four assets, attested 2026-08-15. That is a small sample and it is weighted
toward assets with something to report — see
[Coverage](#what-this-run-does-not-establish). Widening it is in progress.

| Asset | Severity | Base | Escalated | Accountability | Flags |
| --- | --- | --- | --- | --- | --- |
| `AQUA` | 0 clear | 0 | no | verified | `0` |
| `USDC` | 2 medium | 2 | no | unverified | `18` |
| `USDZ` | 3 high | 3 | no | verified | `6` |
| `BERKSHIRE` | 4 critical | 3 | **yes** | unverified | `54` |

---

## AQUA — clear

`AQUA-GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA`
Scanned 2026-09-05T06:55:53Z · attested 2026-08-15T05:26:51Z

**`capability` → clear, mechanics none.** Horizon reports
`auth_required=false auth_revocable=false auth_immutable=false auth_clawback_enabled=false`.
The issuer holds no authorization flags: it cannot freeze, confiscate, or gate
this asset. The reasoning also states the converse, which matters — the flags
are *not* locked (`auth_immutable` unset), so the issuer may add freeze or
confiscation powers later. Under CAP-0035 that would not reach existing
trustlines but would apply to any opened after the change.

**`sep1-domain` → accountability verified.** `home_domain` is `aqua.network`,
and that domain's `stellar.toml` `CURRENCIES` lists this exact code and issuer.
Reciprocal, so a named party has publicly claimed the asset. It contributes
nothing to severity.

**`reputation` → no escalation.** The directory lists the issuer as
`"AQUA Issuer"` (domain `aqua.network`, tags `anchor, issuer`); blocked-domains
returns `blocked=false`. Neither lowers the capability severity.

| | |
| --- | --- |
| On-chain | `severity 0`, `flags 0`, `attested_at 1786771611` |
| `evidence_hash` | `688453bd22e9b694b9c70659d37526bdae18944645542642008e9d961461a4a9` |
| Attest tx | `1b6bafc1226570b2415299f5531256716f4d8dc489a9784fcd6ea347d0f63f5f` |

---

## USDC — medium

`USDC-GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN`
Scanned 2026-09-05T06:55:58Z · attested 2026-08-15T05:27:11Z

**`capability` → medium, mechanics `auth_revocable`.** The issuer can freeze a
holder's balance so it cannot be moved. It **cannot** claw back —
`auth_clawback_enabled=false`, verified live. Worth stating plainly because the
widespread assumption that regulated stablecoins carry clawback is false for the
largest asset on the network.

**`sep1-domain` → accountability unverified, mechanic `domain_unverified`.**
`home_domain` is `circle.com`, and `https://circle.com/.well-known/stellar.toml`
returns **404**. So the most reputable regulated stablecoin on Stellar fails
reciprocal SEP-1 verification by the letter of the spec.

This is the case that justifies keeping accountability out of severity. Had
attribution been a discount, USDC would score worse than a scam asset with a
working `stellar.toml`. It is reported as it is, not special-cased.

Note also that the directory names the issuer `"Centre"` on domain `centre.io`,
which does not match the advertised `circle.com`. Assay does not currently
compare those two — see [tracked as #4](https://github.com/use-assay/Assay/issues/4).

**`reputation` → no escalation.** Directory: `"Centre"` (tags `anchor, issuer`).
Blocked-domains: `circle.com` `blocked=false`.

| | |
| --- | --- |
| On-chain | `severity 2`, `flags 18` (`auth_revocable`\|`domain_unverified`), `attested_at 1786771631` |
| `evidence_hash` | `e7fc765d396de672a85c1b7fe76f67b05a17ad0798167748b285df020ed790bd` |
| Attest tx | `9869957e510ac6cb9ba4e5ffaff66ad0286e420e824efc0095d9e578be37564e` |

---

## USDZ — high, and the reason severity is capability-only

`USDZ-GAKTLPC4ZV37SSCITQ5IS5AQ4WPF4CF4VZJQPPAROSGXMYOATF5U6XPR`
Scanned 2026-09-05T06:56:02Z · attested 2026-08-15T05:27:31Z

**`capability` → high, mechanics `auth_revocable | auth_clawback_enabled`.** The
issuer can freeze a balance and can confiscate it outright and burn it, without
the holder's signature. Clawback already applies to trustlines opened now.

**`sep1-domain` → accountability verified.** `home_domain` is `zeam.money` and
that domain's `stellar.toml` claims this exact code and issuer.

**This is the case the severity model exists for.** USDZ is domain-verified,
unblocklisted, and scores `high` anyway — because a verified domain does not
weaken clawback. It only names who holds it. Accountability is reported beside
severity and never folded into it.

**`reputation` → no escalation.** Directory: `"Zeam.Money"` (domain
`zeam.money`, tags `issuer`). Blocked-domains: `blocked=false`.

A caution, because it would be easy to over-read this asset: a directory tag of
`issuer` and a working `stellar.toml` establish that *someone published a claim*,
not that the entity behind it is what it appears to be. Assay has not
independently verified who operates `zeam.money`, and nothing in this document
should be read as saying it has.

| | |
| --- | --- |
| On-chain | `severity 3`, `flags 6`, `attested_at 1786771651` |
| `evidence_hash` | `ca9b13a66f3a0a4b43d66dea29a505658447e08eee200d2bdb5e66aa1065fb4d` |
| Attest tx | `069d519fa20c472bbc3756291307fb30719219903c1e62a27b4967c0b07ecfd6` |

---

## BERKSHIRE — critical by escalation

`BERKSHIRE-GA22QHSHQEHDJS2ZOINSC77XPPQ24G5EFRJGVEIZLKC5FAW3PQ5XNSDQ`
Scanned 2026-09-05T06:56:07Z · attested 2026-08-15T05:27:46Z

**`capability` → high, mechanics `auth_revocable | auth_clawback_enabled`.**
Identical capability to USDZ. On flags alone these two assets are the same
asset, which is the intended behaviour: capability is capability.

**`sep1-domain` → accountability unverified, mechanic `domain_unverified`.**
`home_domain` is `nasdaq.finance`; DNS does not resolve it.

**`reputation` → escalates to critical, mechanic `blocklisted`.** The curated
directory lists the issuer as `"Scam Asset"` with tags `malicious, unsafe`.
Reported as StellarExpert's determination, not re-derived by Assay, and it
raises the level regardless of the flags.

Base stays `3`; final severity is `4`; `escalated: true`. The two numbers are
stored separately precisely so this is auditable.

**A caveat this asset exposed.** `blocked-domains` returns
`"nasdaq.finance" blocked=false` — the malicious-domain blocklist does **not**
contain a domain whose issuer the directory tags `malicious`. The escalation
came entirely from the directory. Had Assay consulted only the blocklist,
a confirmed scam would not have escalated. Consumed sources are not
interchangeable and are not individually complete.

| | |
| --- | --- |
| On-chain | `severity 4`, `flags 54`, `attested_at 1786771666` |
| `evidence_hash` | `dc2bbf0849dfc001c44be9a5de4fe19c39300e5c4735c9ed3311b3d214171a0d` |
| Attest tx | `322f4bb5fcc84cea9231b7d12e0a964a304a7bd4ea78f97991ca335426e5e2ff` |

---

## The round trip still reproduces, three weeks on

Re-scanned 2026-09-05 against attestations written 2026-08-15. All four
severities, bitsets, and evidence hashes match what the contract stores:

| Asset | Re-scan `severity flags evidence_hash` | On-chain | Match |
| --- | --- | --- | --- |
| `AQUA` | `0 0 688453bd…61a4a9` | `0 0 688453bd…61a4a9` | yes |
| `USDC` | `2 18 e7fc765d…d790bd` | `2 18 e7fc765d…d790bd` | yes |
| `USDZ` | `3 6 ca9b13a6…65fb4d` | `3 6 ca9b13a6…65fb4d` | yes |
| `BERKSHIRE` | `4 54 dc2bbf08…171a0d` | `4 54 dc2bbf08…171a0d` | yes |

That is the property the encoding was designed for: retrieval timestamps are
outside the preimage, so unchanged evidence reproduces the same hash and a
verifier can re-scan rather than trust. One of the four reproduces for a
weaker reason than it appears — see [Finding 2](#finding-2).

## Fail-closed, demonstrated

Verified live on 2026-09-05 using `native` XLM as a control that is deliberately
never attested (`CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC`).

| Attempted | Result | Why that is correct |
| --- | --- | --- |
| `get_safety(unattested)` | `null` | Never-attested stays distinguishable from an attestation of `clear`. Collapsing them would make every unscanned asset read as safe. |
| `is_safe(unattested, max_severity=4, max_age_secs=0)` | `false` | These are the **most permissive arguments the gate accepts** — the maximum severity, freshness check disabled. It still refuses, because there is nothing to admit. |
| gate `would_admit(unattested)` | `false` | Same answer through a real cross-contract call rather than a direct read. |
| gate `deposit(unattested)` **submitted** | reverts `Error(Contract, #1)` `NotAttested` | The refusal is a real transaction revert, not an advisory return value. |

The second row is the one that matters. An unknown asset is unknown, not safe,
and no argument the caller supplies converts one into the other.

---

## What the checks can get wrong

Stated per check, because a scanner that publishes only its successes is not
measurable.

### `capability`

- **Reads one source and does not corroborate it.** Severity comes from the
  `flags` object on Horizon's `/assets` record. The scan also fetches the issuer
  *account*, which carries its own copy of the same flags, and **never compares
  them**. Those are different ingestion paths; if they disagree, Assay silently
  uses one. A free consistency check is available and is not made.
- **Prospective only.** Clawback is inherited at trustline creation (CAP-0035),
  so a `high` verdict describes what happens to a trustline you open *now*, not
  a balance you already hold.
- **`clear` doubles as "not evaluated".** There is no severity value meaning
  unknown, so a report built from an empty subject would read `clear` — the
  safest value in the ABI. Unreachable through the live scanner, which fails
  hard when Horizon fails, but not prevented by construction.
- **No false positives observed in this run.** Flags are consensus-enforced
  booleans; the check reports them without interpretation. Its risk is
  under-reporting, not over-reporting.

### `sep1-domain`

- **`unverified` conflates two different facts.** "The `stellar.toml` could not
  be fetched" and "it was fetched and does not claim this asset" both produce
  `unverified`. The distinction survives only in the prose reasoning and the
  evidence claim — a consumer switching on the JSON `accountability` field
  cannot tell a broken web server from a refused claim. Both appear in this run:
  USDC is a 404, BERKSHIRE is a DNS failure.
- **Linked currency files are not followed.** A `stellar.toml` may point to a
  per-currency file rather than inline the entry. Assay reports that as
  unconfirmed rather than refuted — but the detection counts only entries with a
  `toml` link and *no* code or issuer. An entry carrying both a link and a code
  is missed, so the hedge is skipped in a case where it applies.
- **A verified domain is weak evidence.** It proves someone published a matching
  claim. Publishing a `stellar.toml` takes ten minutes.

### `reputation`

- **This check's failure mode is the subject of [Finding 1](#finding-1) below,
  and it is the most serious thing in this document.**
- **Escalation rests on a self-asserted string.** The blocklist is queried using
  the issuer's `home_domain` — the same free-text field this project elsewhere
  says proves nothing. An issuer that clears `home_domain` is never checked
  against the blocklist at all.
- **Only two directory tags are recognised** (`malicious`, `unsafe`). Any other
  adverse tag is silently ignored — a false negative that will never announce
  itself.
- **Consumed sources are not individually complete**, as BERKSHIRE showed:
  directory `malicious`, blocklist `blocked=false`.

---

## Findings from this run

### Finding 1 — a source outage is reported as a clean result {#finding-1}

`internal/stellarexpert/client.go` correctly separates outcomes: HTTP 404 means
genuinely not listed and returns no error; 429, 5xx, and timeouts return an
error. `internal/scan/scan.go` then discards that error:

```go
if blocked, err := s.Expert.BlockedDomain(ctx, domain); err == nil { sub.Blocked = blocked }
if entry, err := s.Expert.Directory(ctx, a.Issuer); err == nil { sub.Directory = entry }
```

Both outcomes leave the field `nil`, and `check_reputation.go` then reports:

> "No curated reputation data was available for this issuer. **That is the
> normal case** and is not a positive signal."

That sentence is true on a 404 and false on a rate-limit. The report cannot tell
them apart, and neither can anything downstream — no field on the report records
that a source was unreachable.

**Why it matters.** `BERKSHIRE` above reaches `critical` *only* through
reputation; its capability severity is `high`, and `DOGE-GA22IDJN…` — a known
scam in the eval set — is `clear` on capability and `critical` only by
escalation. During a StellarExpert outage that asset scans `clear`, and the
resulting report passes the attestation invariant and is writable on-chain as
clear.

It contradicts this project's own published rule in
[docs/checks.md](checks.md): *"'We could not check' and 'this is fine' are
different answers and must never render the same."* Ledger data fails closed —
Horizon errors abort the scan — while reputation fails open and silent. That
asymmetry was not documented anywhere.

Per [CONTRIBUTING.md](../CONTRIBUTING.md), a way to make Assay under-report risk
is a security issue rather than a bug.

**Status: open**, being fixed next. This run's four assets are unaffected: all
three sources answered for all four, evidenced by the four evidence entries each
report carries. But that is luck rather than design — nothing in the output
would have told us otherwise, which is the finding.

### Finding 2 — the evidence hash is not reproducible across machines {#finding-2}

`evidence_hash` is documented as verifiable by re-scanning and recomputing. That
holds only when the evidence text is machine-independent, and it is not.

When a `stellar.toml` fetch fails, the raw Go transport error is recorded
verbatim and hashed. BERKSHIRE's preimage — one of the four already on-chain —
contains:

```
evidence	stellar.toml	https://nasdaq.finance/.well-known/stellar.toml	not retrievable: sep1: fetch https://nasdaq.finance/.well-known/stellar.toml: Get "https://nasdaq.finance/.well-known/stellar.toml": dial tcp: lookup nasdaq.finance on 10.255.255.254:53: no such host
```

`10.255.255.254` is **this machine's DNS resolver**. The on-chain hash
`dc2bbf08…171a0d` therefore commits to a local network detail. It reproduces
above only because the re-scan ran on the same host; a verifier anywhere else
gets different bytes, a different hash, and would reasonably conclude the
attestation does not match its evidence when nothing is wrong.

The same class of text — timeouts carrying resolved IPs, `context deadline
exceeded` versus a connection reset — makes any asset with an unreachable domain
non-reproducible. USDC is unaffected: a clean `status 404` is stable.

**Status: not fixed, deliberately.** The fix is to normalise transport errors to
a stable form before they enter the preimage, which changes the preimage and so
requires an `assay-evidence-v2` version bump and re-attestation of BERKSHIRE.
That is a larger change than it looks and is not being rushed into the same
sitting that found it. Tracked as an issue.

---

## What this run does not establish

- **Four assets is not a measurement.** No precision or recall number is quoted,
  because four subjects cannot support one.
- **The sample is skewed toward findings.** Three of four assets have something
  to report. A scanner evaluated only on assets with problems is uncalibrated;
  widening the sample with assets expected to come back clean is in progress and
  this document will grow.
- **No independent verification of any issuer's identity.** A verified
  `stellar.toml` and a directory listing are claims by others, recorded as
  attributed evidence. Assay has not confirmed that any issuer here is the
  entity it appears to be.
- **Testnet.** One key can write any attestation; testnet resets and Soroban TTL
  expiry will remove these entries.
- **Attested 2026-08-15, re-verified 2026-09-05.** Issuers can change flags at
  any time. These attestations are exactly as fresh as their `attested_at`, and
  nothing refreshes them on a schedule.
