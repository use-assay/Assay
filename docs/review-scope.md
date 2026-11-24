# External security review scope

What an external reviewer of the Soroban contracts should read, which
entrypoints and invariants are in scope, and what is deliberately excluded. It
exists so a review has a scope rather than a whole repository: the judging
layer (severity assignment, reputation consumption, the scanner) is a different
system with a different threat model, and conflating the two hides the place
where an error costs money.

This document is **about the contracts**, not the Go scanner. The scanner's
trust assumptions are in [threat-model.md](threat-model.md); the contract's
interface is [contract-interface.md](contract-interface.md).

## The review package

| Item | Where it is |
| --- | --- |
| Source under review | `assay-contracts/contracts/safety-registry/` at the commit on the review's pull request (`git rev-parse HEAD`) |
| Threat model | [threat-model.md](threat-model.md) — actors, attack classes, verdicts |
| Contract interface | [contract-interface.md](contract-interface.md) — entrypoints, ABI, the evidence preimage |
| Deployment record and deployed wasm hashes | [deployment.md](deployment.md) — the registry is `c4105b91b3ceae95b5a225c55bc6981b3dcf71d07fdd0ee5c79a21d25edd301b`, the example gate is `ca40172ec8ebc259e7e21429748b2bd7cafe467fa3005d849d81d16938e8987c` |
| In-scope invariants | this document, [below](#invariants-and-their-tests) |
| Tests | `make contract-test`, and the named functions in `assay-contracts/contracts/safety-registry/src/test.rs` and `src/test/fail_closed.rs` |

### Reproducing the artifact

```sh
make contract-build                 # plain compile, no optimizer
sha256sum assay-contracts/out/*.wasm
make verify-wasm                    # rebuild from committed source, compare to deployment.md
```

Record the `sha256sum` **and the toolchain** in the review ticket. The toolchain
is pinned in [`assay-contracts/rust-toolchain.toml`](../assay-contracts/rust-toolchain.toml),
and the build is deterministic under one compiler: the same source and the same
`rustc` produce byte-identical wasm, while a different `rustc` produces different
bytes from identical source. A hash is therefore meaningful only with the
toolchain that produced it.

The deployment's recorded hashes are historical. They were produced before the
toolchain was pinned, so current source does not reproduce them and a mismatch
is **expected** — it is [#127](https://github.com/use-assay/Assay/issues/127),
not evidence that the source drifted. The measurement is in
[deployment.md](deployment.md#does-the-source-still-build-what-is-deployed). Do
not treat the deployed hash as the thing under review; reviewing the source on
the pull request is the primary artifact, and the hash is corroboration.

## In scope

The deployed registry is **testnet only and not a candidate for mainnet**
([threat-model.md](threat-model.md#what-this-document-does-not-authorize)).
Review the following as the contract that a gate may rely on:

| Entrypoint | Authorization | What it does |
| --- | --- | --- |
| `init(admin)` | none, single-shot | Sets the admin; refuses a second call |
| `attest(asset, severity, flags, evidence_hash, attested_at, preimage_version)` | admin `require_auth` | Validates and writes one attestation, publishes `Attested` |
| `attest_many(Vec<Attestation>)` | admin `require_auth` | Validates every element through the same function, then writes all or nothing; bounded by `MAX_BATCH_SIZE` |
| `revoke(asset)` | admin `require_auth` | Removes one attestation, publishes `Revoked`; refuses a non-attested asset |
| `get_safety(asset)` | none | Returns `Option<Safety>`; `None` for unattested and revoked |
| `get_safety_many(Vec<Address>)` | none | Bounded batch read, order-preserving, `None` per missing asset |
| `get_attestation(attestor, asset)` | none | Returns that attestor's `Safety` for the asset |
| `is_safe(asset, max_severity, max_age_secs)` | none | Fail-closed gate on severity, flags and freshness |
| `is_safe_masked(asset, max_severity, mask, max_age_secs)` | none | Fail-closed gate on the caller's forbidden-bit mask |
| `quorum(...)` | none | Conservative aggregation over attestors |
| `is_safe_quorum(...)` | none | Fail-closed gate over the quorum result |
| `transfer_admin(new_admin)` / `accept_admin()` / `cancel_admin_transfer()` | current admin / pending admin | Two-step rotation |
| `admin()` / `pending_admin()` | none | Rotation state |
| `version()` | none | `INTERFACE_VERSION` |

Out of scope for the review: the Go scanner, the severity judging layer, the
attestation writer pipeline, deployment scripts, and the HTTP API. Their
failures can produce a wrong *input* to `attest`, which the contract cannot
detect — attack class 1 in [threat-model.md](threat-model.md#1-attester-writes-clear-for-a-dangerous-asset).

## Invariants and their tests

Where an invariant has no test, it is **flagged** rather than listed as
covered. A review scope that presents untested properties as covered is worse
than no scope.

| Invariant | Test(s) | Notes |
| --- | --- | --- |
| An unattested asset reads `None`, and every gate refuses it at every threshold and mask | `unattested_asset_returns_none`, `gate_fails_closed_on_unattested_asset`, `masked_gate_fails_closed_on_unattested_asset`, `fail_closed_unattested_refused_at_every_threshold_and_mask` | `None` is never coerced to a default severity |
| A revoked asset reads `None`, and a later read cannot resurrect it | `get_safety_many_revoked_reads_as_none`, `admin_revokes_attestation`, `revoke_leaves_other_assets_attested`, `revoke_then_reattest` | Revoked and never-attested are indistinguishable on purpose |
| A stale attestation is refused at every threshold, in both gate shapes | `stale_attestation_fails_closed`, `masked_gate_stale_attestation_fails_closed`, `fail_closed_stale_refused_at_every_threshold` | `max_age_secs` is the caller's policy, not a constant |
| Confiscation cannot be attested below `high`: the clawback bit implies `severity >= SEVERITY_HIGH` | `attest_rejects_clawback_below_high`, `gate_blocks_confiscation_below_high_threshold`, `masked_gate_blocks_confiscation_capable_asset`, `fail_closed_gate_refuses_confiscation_below_high_even_if_stored`, `fail_closed_gate_refuses_confiscation_when_the_write_time_check_is_bypassed` | Enforced at write **and** re-checked at read, so bypassing the write check does not open the gate |
| Severity is inside `clear..=critical` | `attest_rejects_out_of_range_severity` | Out-of-range is rejected, never clamped |
| An asset argument must be a contract address that is neither the registry nor the admin | `attest_rejects_the_registrys_own_address`, `attest_rejects_the_admin_address`, `attest_rejects_a_non_contract_address`, `batch_with_an_invalid_asset_address_writes_nothing` | Which contract really is the SAC for `CODE-ISSUER` is **flagged**: deliberately the caller's responsibility, asserted by nothing on-chain ([contract-interface.md](contract-interface.md#assets-are-sac-addresses)) |
| Only the admin can write or revoke; only the admin can start a transfer; only the pending admin can accept it | `attest_rejects_no_auth_caller`, `attest_many_rejects_unauthorized_caller`, `revoke_rejects_unauthorized_caller`, `transfer_admin_rejects_unauthorized_caller`, `cancel_admin_transfer_requires_the_current_admin`, `accept_admin_rejects_a_caller_other_than_the_pending_admin` | |
| `init` is single-shot | `init_is_single_shot` | The admin cannot be replaced by re-initializing |
| A batch is all-or-nothing and bounded by `MAX_BATCH_SIZE` | `batch_with_one_invalid_entry_writes_nothing`, `batch_over_the_cap_is_rejected`, `batch_of_exactly_the_cap_is_accepted`, `empty_batch_succeeds_and_writes_nothing`, `attest_many_is_all_or_nothing_over_generated_batches`, `batch_accepts_what_the_single_path_accepts` | No partial batch can leave an inconsistent view |
| The single and batch write paths accept exactly the same inputs | `batch_accepts_what_the_single_path_accepts`, `batch_reads_match_single_write_path` | Both call `validate_attestation`; the equivalence is asserted, not assumed |
| The batch bound is measured against live limits with headroom | `batch_size_bound_is_measured_and_holds_headroom` | The binding limit is the contract-event budget |
| `get_safety_many` is bounded and preserves order and `None` for missing assets | `get_safety_many_over_the_bound_is_rejected`, `get_safety_many_at_the_bound_is_accepted`, `get_safety_many_preserves_order_and_none_for_missing`, `get_safety_many_revoked_reads_as_none` | Rejected above the bound, never silently truncated |
| A rejected write publishes no event; a successful write publishes exactly one per asset | `attest_emits_event_on_success`, `attest_publishes_no_event_on_rejection`, `revoke_emits_event_on_success`, `revoke_publishes_no_event_on_rejection`, `batch_emits_one_event_per_element` | Events are published after the write |
| Admin rotation is two-step, observable, and cannot be shortcut | `transfer_admin_emits_admin_transfer_proposed`, `accept_admin_emits_admin_transferred`, `second_transfer_admin_nomination_supersedes_the_first`, `accept_admin_consumes_the_pending_nomination`, `accept_admin_without_a_pending_nomination_returns_no_pending_admin`, `attestations_survive_admin_transfer_and_follow_the_new_admin`, `admin_and_pending_admin_getters_reflect_lifecycle` | |
| Storage TTL is extended on write to the network maximum; reads do not extend it | `attest_extends_ttl_to_network_max`, `batch_extends_ttl_to_network_max`, `ttl_init_extends_instance_to_network_max`, `ttl_revoke_extends_instance_to_network_max`, `ttl_admin_transfer_entrypoints_extend_instance_to_network_max`, `reattest_renews_ttl`, `reads_do_not_extend_ttl` | Archival is not expiry: `read_after_archival_returns_original_attestation` |
| An attestation from a future timestamp cannot underflow the age check | `attestation_from_the_future_does_not_underflow` | |
| Re-attesting identical evidence moves only the timestamp | `re_attestation_identical_evidence_moves_only_timestamp` | `evidence_hash` is stored, never verified against the caller |
| Quorum aggregation is conservative and cannot be inflated | `quorum_deduplicates_attestors_and_cannot_inflate_threshold`, `quorum_contested_and_unavailable_are_distinguishable_on_chain`, `quorum_conservative_aggregation_one_honest_attestor_prevents_admission`, `quorum_conservative_aggregation_property` | |
| Gates return a boolean and never panic on any attested/absent combination | `is_safe_property_unattested`, `is_safe_property_exhaustive` | Property tests over the gate surface |
| The ABI surface that consumers bind to does not drift silently | `error_code_values_are_abi`, `interface_version_is_abi`, `version_returns_interface_version` | Error codes and `INTERFACE_VERSION` are ABI |
| Entrypoint costs stay within the recorded baseline | `entrypoint_costs_are_within_the_recorded_baseline`, `get_safety_many_cost_at_the_bound`, `quorum_cost_at_the_bound` | Recorded in `assay-contracts/contracts/safety-registry/cost-baseline.json` |
| The deployed contract has no upgrade entrypoint | **Flagged — no test asserts it.** | Immutability is structural: there is no entrypoint, and no test enumerates the exported surface to prove its absence |

## Out of scope

This list mirrors [threat-model.md](threat-model.md#out-of-scope). If the two
disagree, the threat model wins and this document is the bug.

| Out of scope | Why |
| --- | --- |
| Whether a `clear` asset is a scam, worthless, or an impersonation | Severity measures issuer power over holders, not fraud in general ([severity-model.md](severity-model.md#what-severity-does-not-tell-you)) |
| Exposure of an *existing* holder's trustline | Attestations are prospective: what a new trustline inherits (CAP-0035) |
| The scanner host or attester key being compromised | Everything downstream of a compromised host is attacker-controlled; the on-chain consequence is threat class 1 |
| Consumer-side mishandling (wrong SAC, wrong `max_age_secs`, misreading `None`) | The safe answer is the default; a consumer that unwraps `None` has left the model ([integrating.md](integrating.md)) |
| Stellar consensus, Horizon correctness, ledger reversion | Upstream of every claim here |
| Testnet resets | Testnet state is periodically removed |
| MEV, front-running or order flow around a gated action | The gate answers whether the asset is safe to hold, not the trade terms |
| Reputation data accuracy | Consumed and attributed, not curated; report it upstream ([SECURITY.md](../SECURITY.md)) |
| Denial of service against Assay's HTTP API | Not deployed as a service; fetch-layer unbounded reads **are** in scope for the scanner, not the contract |

## Known accepted risks

These are accepted, not oversights, and are argued in
[threat-model.md](threat-model.md). A review should test the *bounds* of each,
not report the class as a new finding:

1. **A compromised or malicious attester can write `clear` for a dangerous
   asset** (class 1). `evidence_hash` is detective, not preventive. The fix is
   threshold attestation, designed in [multi-attestor.md](multi-attestor.md).
2. **An issuer can change flags after the attestation** (class 2). Mitigated
   only by the consumer's `max_age_secs`, measured in [freshness.md](freshness.md).
3. **A wrong listing escalates; a missing listing never reassures** (class 9).
   The failure direction is asymmetric in Assay's favour.
4. **The attester can stop writing, or revoke a correct attestation** (classes
   10 and 11). Both fail closed.
5. **Permanent admin takeover during the pending-admin window** (class 14).
   Accepted; recovery is a fresh registry, documented in
   [attester-key.md](attester-key.md#response-to-compromise).
6. **A consumer reading only the current attestation sees a lowered severity
   with no on-chain record that it was ever higher** (class 4 residual). Off
   chain, the history API now emits a typed, attributed de-escalation
   ([transitions.md](transitions.md#de-escalation)); the on-chain marker is
   designed in [ever-escalated-marker.md](ever-escalated-marker.md) and is not
   implemented. This is why the contract is testnet-only.

## Running the checks

```sh
make contract-test     # cargo test: every test named above
make contract-lint     # clippy, with warnings as errors
make contract-build    # wasm build
make verify-wasm       # build from committed source and compare with deployment.md
```

The test names in this document are function names in
`assay-contracts/contracts/safety-registry/src/test.rs` and
`src/test/fail_closed.rs`; `cargo test <name>` runs one.
