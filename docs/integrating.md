# Calling `get_safety` from your contract

How a Soroban contract reads an Assay attestation and refuses to act on an asset
whose issuer can take it back.

Everything here runs against the live testnet deployment:

```
CBK4FBIHMDTXCUPE4E3ZDVSFJSCY5FJETTKNIQPN4LFJIKKIBLKIXQ73
```

The complete, compiling version of this guide is
[`contracts/example-gate`](../assay-contracts/contracts/example-gate) — it is a
workspace crate with tests, built and deployed to testnet at
`CANO57JRGTATHGLM26TWYPIXERSPVI5R52H33K7ZUJGGOEOVVZA44W3U`. Read
[deployment.md](deployment.md) for the transaction hashes.

## What you are trusting

Before the code: the registry is an **attestation store**, not a scanner. A
Soroban contract cannot fetch Horizon mid-transaction, so it cannot read an
issuer's flags itself. What it can do is read, atomically, what an off-chain
scanner concluded — which is the half that actually needs to be on-chain, since
an HTTP call before you submit can be answered against different ledger state
than the one your transaction lands in.

So you are trusting the attester. Today that is one key. `evidence_hash` is what
makes the trust checkable rather than absolute: it commits to the exact evidence
the scanner read, and anyone can re-scan and recompute it. See
[contract-interface.md](contract-interface.md).

## 1. Declare the interface

You do not need a dependency on Assay. Declare the parts you call:

```rust
use soroban_sdk::{contractclient, contracttype, Address, BytesN, Env};

/// Field names and types are ABI — they must match the registry exactly,
/// because a #[contracttype] struct crosses the boundary as a map keyed by
/// these names.
#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct Safety {
    pub severity: u32,
    pub flags: u32,
    pub evidence_hash: BytesN<32>,
    pub attested_at: u64,
}

#[contractclient(name = "SafetyRegistryClient")]
pub trait SafetyRegistry {
    fn get_safety(env: Env, asset: Address) -> Option<Safety>;
    fn is_safe(env: Env, asset: Address, max_severity: u32, max_age_secs: u64) -> bool;
}
```

## 2. Decide what you are refusing

The bits, which are ABI and mirror `internal/mechanics`:

| Bit | Mechanic | What the issuer can do |
| --- | --- | --- |
| `1 << 0` | `auth_required` | Decide who may open a trustline |
| `1 << 1` | `auth_revocable` | Freeze an existing holder's balance |
| `1 << 2` | `auth_clawback_enabled` | Confiscate and burn a balance |
| `1 << 3` | `auth_immutable` | Nothing — the flag set can never change again |
| `1 << 4` | `domain_unverified` | Nothing — no verified party claims the asset |
| `1 << 5` | `blocklisted` | Nothing — a curated source flagged the issuer |

Severity `0..=4` is the same information collapsed to one ordered number:
`0` clear, `1` `auth_required`, `2` `auth_revocable`, `3`
`auth_clawback_enabled`, `4` reputation escalation.

**Gate on the bitset when you care about a specific power.** Severity answers
"how bad"; the bitset answers "which power", and a contract usually has an
opinion about a particular one. The difference is not hypothetical — from the
live deployment:

- `USDC` is attested at severity `2` with flags `18`
  (`auth_revocable | domain_unverified`). A gate reading `severity <= 2` admits
  it. A custodial balance that cannot survive a freeze should not.
- `USDZ` is attested at severity `3` with flags `6`
  (`auth_revocable | auth_clawback_enabled`), and its domain *is* verified.
  Verification does not weaken clawback, so it is still refused.

Note also that bits `1 << 3` through `1 << 5` are reported, not powers. Refusing
on `domain_unverified` refuses most of the network, including plenty of assets
whose issuers can do nothing to you.

## 3. Write the gate

```rust
pub const MECH_AUTH_REVOCABLE: u32 = 1 << 1;
pub const MECH_CLAWBACK_ENABLED: u32 = 1 << 2;

/// Powers a custodial balance cannot survive.
pub const REFUSED_MECHANICS: u32 = MECH_AUTH_REVOCABLE | MECH_CLAWBACK_ENABLED;

/// Your policy, not Assay's. A deposit gate and a large settlement should not
/// be forced to agree on how fresh is fresh enough.
pub const MAX_ATTESTATION_AGE: u64 = 24 * 60 * 60;

fn assert_safe(env: &Env, registry: &Address, asset: &Address) -> Result<(), Error> {
    let registry = SafetyRegistryClient::new(env, registry);

    // Handle None first and explicitly. An asset nobody has attested is
    // unknown, not safe — and treating unknown as safe would let any asset
    // through by the simple expedient of never being scanned.
    let Some(safety) = registry.get_safety(asset) else {
        return Err(Error::NotAttested);
    };

    if env.ledger().timestamp().saturating_sub(safety.attested_at) > MAX_ATTESTATION_AGE {
        return Err(Error::AttestationStale);
    }

    if safety.flags & REFUSED_MECHANICS != 0 {
        return Err(Error::IssuerCanTakeIt);
    }

    Ok(())
}
```

Then call it before you act, in the same transaction:

```rust
pub fn deposit(env: Env, asset: Address, from: Address, amount: i128) -> Result<(), Error> {
    from.require_auth();
    let registry: Address = env.storage().instance().get(&DataKey::Registry).unwrap();
    assert_safe(&env, &registry, &asset)?;
    // ... credit the balance
    Ok(())
}
```

### Or let the registry decide

If a severity threshold really is your whole policy, `is_safe` is the one-liner,
and it fails closed on every path — never attested, stale, too severe, or
internally inconsistent:

```rust
if !registry.is_safe(&asset, &2, &3600) {
    return Err(Error::AssetNotSafe);
}
```

`max_age_secs = 0` disables the freshness check. Pass it only if you have
decided that staleness is acceptable, not to make a test go green.

## 4. Deploy against the live registry

Take the registry address as a constructor argument rather than hardcoding it,
so the same wasm works on both networks:

```rust
pub fn __constructor(env: Env, registry: Address) {
    env.storage().instance().set(&DataKey::Registry, &registry);
}
```

```sh
stellar contract deploy \
  --wasm target/wasm32v1-none/release/your_contract.wasm \
  --source-account your-key --network testnet \
  -- --registry CBK4FBIHMDTXCUPE4E3ZDVSFJSCY5FJETTKNIQPN4LFJIKKIBLKIXQ73
```

## 5. Look up an asset's address

The registry keys on an asset's **Stellar Asset Contract address**, not on
`CODE:ISSUER`. Address comparison is cheap on-chain; string handling is not.

```sh
stellar contract id asset --asset AQUA:GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA \
  --network testnet
# CDJF2JQINO7WRFXB2AAHLONFDPPI4M3W2UM5THGQQ7JMJDIEJYC4CMPG
```

That address is derived from the network passphrase, so **the same asset has a
different address on testnet than on pubnet**. Derive it for the network you are
deploying to; do not copy one across.

## Trying it against the live deployment

The four attested assets, and one that is deliberately never attested:

```sh
GATE=CANO57JRGTATHGLM26TWYPIXERSPVI5R52H33K7ZUJGGOEOVVZA44W3U

# AQUA — attested clear
stellar contract invoke --id $GATE --source-account your-key --network testnet --send=no \
  -- would_admit --asset CDJF2JQINO7WRFXB2AAHLONFDPPI4M3W2UM5THGQQ7JMJDIEJYC4CMPG
# true

# USDZ — attested high, issuer can claw back
stellar contract invoke --id $GATE --source-account your-key --network testnet --send=no \
  -- would_admit --asset CAOM5NKBTSGEXTZZKH3STWSFWURMODC3TZ4NS2THN7W5YUDFK3IOHIHU
# false

# native XLM — never attested
stellar contract invoke --id $GATE --source-account your-key --network testnet --send=no \
  -- would_admit --asset CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC
# false
```

Submitting a `deposit` rather than simulating gives you the reason: `USDZ`
reverts with `Error(Contract, #3)` (`IssuerCanTakeIt`) and the unattested asset
with `Error(Contract, #1)` (`NotAttested`).

## Verifying an attestation yourself

You do not have to take the stored severity on faith. Re-scan the asset and
recompute the hash:

```sh
$ ./assay attestation -raw USDZ-GAKTLPC4ZV37SSCITQ5IS5AQ4WPF4CF4VZJQPPAROSGXMYOATF5U6XPR
3	6	ca9b13a66f3a0a4b43d66dea29a505658447e08eee200d2bdb5e66aa1065fb4d

$ make read ASSET=USDZ-GAKTLPC4ZV37SSCITQ5IS5AQ4WPF4CF4VZJQPPAROSGXMYOATF5U6XPR
{"attested_at":...,"evidence_hash":"ca9b13a6...65fb4d","flags":6,"severity":3}
```

`assay attestation -preimage` prints the exact bytes hashed, so the check can be
reimplemented in any language. The encoding is specified in
[contract-interface.md](contract-interface.md).

If the hashes differ, either the asset's sources changed since the attestation
or the attestation does not correspond to the evidence it claims. The hash
cannot tell you which — that is what `attested_at` and your own re-scan are for.

## Before you rely on this

- It is on **testnet**, not pubnet.
- **Four assets are attested.** Everything else returns `None`, which your gate
  must treat as "unknown", and which — if you gate correctly — means your
  contract refuses nearly every asset on the network.
- **One key can write any attestation.** There is no multisig and no threshold
  of independent attesters yet.
- **Nothing refreshes the attestations.** They are exactly as fresh as their
  `attested_at`. Choose a `max_age_secs` you would actually accept.
- Testnet is periodically reset, and Soroban persistent entries expire if their
  TTL is not extended. Either will remove these attestations.
