#![cfg(test)]

use super::*;
use soroban_sdk::{testutils::Address as _, testutils::Ledger as _, BytesN, Env};

fn setup() -> (Env, SafetyRegistryClient<'static>, Address) {
    let env = Env::default();
    env.mock_all_auths();

    let contract_id = env.register(SafetyRegistry, ());
    let client = SafetyRegistryClient::new(&env, &contract_id);
    let admin = Address::generate(&env);
    client.init(&admin);

    (env, client, admin)
}

fn hash(env: &Env) -> BytesN<32> {
    BytesN::from_array(env, &[7u8; 32])
}

#[test]
fn unattested_asset_returns_none() {
    let (env, client, _) = setup();
    let asset = Address::generate(&env);

    assert_eq!(client.get_safety(&asset), None);
}

/// The most important test in this contract. An asset nobody has ever scanned
/// must never be treated as safe.
#[test]
fn gate_fails_closed_on_unattested_asset() {
    let (env, client, _) = setup();
    let asset = Address::generate(&env);

    assert!(!client.is_safe(&asset, &SEVERITY_CRITICAL, &0));
    assert!(!client.is_safe(&asset, &SEVERITY_CLEAR, &0));
}

#[test]
fn attest_then_read_roundtrips() {
    let (env, client, _) = setup();
    let asset = Address::generate(&env);
    env.ledger().set_timestamp(1_000);

    client.attest(&asset, &SEVERITY_MEDIUM, &MECH_AUTH_REVOCABLE, &hash(&env));

    let got = client.get_safety(&asset).expect("attestation should exist");
    assert_eq!(got.severity, SEVERITY_MEDIUM);
    assert_eq!(got.flags, MECH_AUTH_REVOCABLE);
    assert_eq!(got.attested_at, 1_000);
    assert_eq!(got.evidence_hash, hash(&env));
}

#[test]
fn gate_admits_within_threshold_and_blocks_above() {
    let (env, client, _) = setup();
    let asset = Address::generate(&env);

    client.attest(&asset, &SEVERITY_MEDIUM, &MECH_AUTH_REVOCABLE, &hash(&env));

    assert!(client.is_safe(&asset, &SEVERITY_MEDIUM, &0));
    assert!(client.is_safe(&asset, &SEVERITY_HIGH, &0));
    assert!(!client.is_safe(&asset, &SEVERITY_LOW, &0));
    assert!(!client.is_safe(&asset, &SEVERITY_CLEAR, &0));
}

#[test]
fn stale_attestation_fails_closed() {
    let (env, client, _) = setup();
    let asset = Address::generate(&env);

    env.ledger().set_timestamp(1_000);
    client.attest(&asset, &SEVERITY_CLEAR, &0, &hash(&env));
    assert!(client.is_safe(&asset, &SEVERITY_MEDIUM, &600));

    env.ledger().set_timestamp(1_000 + 601);
    assert!(!client.is_safe(&asset, &SEVERITY_MEDIUM, &600));

    // max_age_secs = 0 disables the freshness requirement.
    assert!(client.is_safe(&asset, &SEVERITY_MEDIUM, &0));
}

#[test]
fn attestation_from_the_future_does_not_underflow() {
    let (env, client, _) = setup();
    let asset = Address::generate(&env);

    env.ledger().set_timestamp(5_000);
    client.attest(&asset, &SEVERITY_CLEAR, &0, &hash(&env));
    env.ledger().set_timestamp(1_000);

    assert!(client.is_safe(&asset, &SEVERITY_MEDIUM, &600));
}

/// Confiscation capability must not be expressible below High. The writer
/// rejects it so a reader can rely on the invariant.
#[test]
fn attest_rejects_clawback_below_high() {
    let (env, client, _) = setup();
    let asset = Address::generate(&env);

    let err = client
        .try_attest(
            &asset,
            &SEVERITY_MEDIUM,
            &MECH_CLAWBACK_ENABLED,
            &hash(&env),
        )
        .expect_err("clawback below high must be rejected");

    assert_eq!(err, Ok(Error::InconsistentAttestation));
}

#[test]
fn attest_rejects_out_of_range_severity() {
    let (env, client, _) = setup();
    let asset = Address::generate(&env);

    let err = client
        .try_attest(&asset, &(SEVERITY_CRITICAL + 1), &0, &hash(&env))
        .expect_err("severity above critical must be rejected");

    assert_eq!(err, Ok(Error::InvalidSeverity));
}

/// Even if a bad attestation somehow existed, a gate below High must not admit
/// a confiscation-capable asset.
#[test]
fn gate_blocks_confiscation_below_high_threshold() {
    let (env, client, _) = setup();
    let asset = Address::generate(&env);

    client.attest(
        &asset,
        &SEVERITY_HIGH,
        &(MECH_CLAWBACK_ENABLED | MECH_AUTH_REVOCABLE),
        &hash(&env),
    );

    assert!(!client.is_safe(&asset, &SEVERITY_MEDIUM, &0));
    assert!(client.is_safe(&asset, &SEVERITY_HIGH, &0));
}

#[test]
fn init_is_single_shot() {
    let (env, client, _) = setup();
    let other = Address::generate(&env);

    let err = client.try_init(&other).expect_err("second init must fail");
    assert_eq!(err, Ok(Error::AlreadyInitialized));
}

/// attest() requires auth from the admin set at init time. A non-admin
/// caller must be rejected. This test does not use mock_all_auths(), so
/// require_auth() on the admin address actually enforces.
#[test]
fn attest_rejects_unauthorized_caller() {
    let env = Env::default();
    let contract_id = env.register(SafetyRegistry, ());
    let client = SafetyRegistryClient::new(&env, &contract_id);
    let admin = Address::generate(&env);
    client.init(&admin);

    let _caller = Address::generate(&env);
    let asset = Address::generate(&env);

    let err = client
        .try_attest(&asset, &SEVERITY_CLEAR, &0, &hash(&env))
        .expect_err("non-admin must be rejected");

    // The error type is SDK-internal (soroban_sdk::Error), not our contract
    // Error enum. The important property is that it is an error at all: a
    // non-admin caller must not be able to write attestations.
    assert!(err.is_err());
}

/// Verifies host behavior for archived persistent entries. An archived entry
/// is one whose TTL has expired (live_until < current ledger sequence).
/// Per CAP-0066 / Protocol 23, the host automatically restores archived entries
/// when they are accessed during a transaction (including simulation). The
/// restored entry receives a fresh TTL (current_sequence + min_persistent_ttl - 1).
/// This test documents the actual behavior: archived entries return the data
/// (Some(Safety)) with the original attested_at timestamp, making them
/// distinguishable from never-attested entries (which return None).
#[test]
fn archived_entry_auto_restored_and_readable() {
    let (env, client, _) = setup();
    let asset = Address::generate(&env);

    // Write an attestation at ledger 0 (default sequence)
    env.ledger().set_timestamp(1_000);
    client.attest(&asset, &SEVERITY_CLEAR, &0, &hash(&env));

    // Verify it's readable initially
    let initial = client.get_safety(&asset);
    assert!(
        initial.is_some(),
        "entry should be readable before archival"
    );
    let original_attested_at = initial.unwrap().attested_at;

    // Default min_persistent_entry_ttl is 4096, so live_until = 4095.
    // Advance sequence to 4096 to expire the entry.
    env.ledger().set_sequence_number(4096);

    // Read the archived entry via contract function.
    // In test mode with soroban-sdk 27.0.5, the host auto-restores the entry
    // and extends its TTL. The original attested_at is preserved.
    let archived = client.get_safety(&asset);

    // Archived entries are distinguishable from never-attested:
    // - Never attested: get_safety returns None
    // - Archived: get_safety returns Some(Safety) with original attested_at
    assert!(
        archived.is_some(),
        "archived entry should be auto-restored and readable"
    );
    assert_eq!(
        archived.unwrap().attested_at,
        original_attested_at,
        "attested_at preserved after restoration"
    );

    // The entry now has a fresh TTL (live_until = 4096 + 4095 = 8191)
    // This is verified by the test snapshot.
}
