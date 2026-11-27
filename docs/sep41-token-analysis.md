# What can be concluded mechanically about a SEP-41 (non-SAC) token

**Status:** research finding, 2026-11-23.
**Scope:** Soroban-native SEP-41 tokens that are *not* Stellar Asset Contracts
(SACs), the admin functions common in deployed examples, and what Assay can
honestly conclude from WASM or an interface probe without trusting the
deployer.

## Why this is a different problem from classic assets

Assay classifies a classic asset from its issuer's authorization flags. Those
flags are consensus-enforced ledger state: `auth_required`,
`auth_revocable`, `auth_clawback_enabled` and `auth_immutable` describe what
the issuer can do, and every SAC exposes the same administrative interface
(`set_admin`, `set_authorized`, `clawback`).

A SEP-41 contract token has no issuer account and no flags. It is a WASM
contract whose admin powers exist only as functions someone chose to export.
[SEP-41][sep41] defines the token interface — `allowance`, `approve`,
`balance`, `transfer`, `transfer_from`, `burn`, `burn_from`, `decimals`,
`name`, `symbol` — and deliberately does **not** define `mint`, `clawback`,
`set_authorized` or any admin surface. The SEP says implementations may name
and gate those functions however they like, requiring only that a mint emits a
`mint` event and a clawback emits a `clawback` event.

Pointing an `Address`-keyed classic-asset gate at one of these contracts
therefore says nothing about its admin powers. Today Assay reports such a
token as unattested, which is correct. This note records what could ever be
said mechanically.

## Survey of deployed SEP-41 tokens

Five mainnet contracts were inspected from their StellarExpert metadata, plus
source where it is published. StellarExpert reports the detected interface
feature (`Traits: sep41`) from the deployed WASM, not from a project's claim.

| Token | Contract | What was observed | Admin surface |
| --- | --- | --- | --- |
| KALE-STROOP Soroswap LP | [`CA2TZIB56KYKD46F7IFBF6XPO5TDNK6N2U6BRTGZ5AF4WUSBN6BKZMGF`][sx-soroswap] | WASM contract, `Traits: sep41`, source unavailable | The published pair source exports the SEP-41 interface; `internal_mint`/`internal_burn` are internal and are called by the pair on liquidity add/remove. No exported `clawback`, `freeze`, `set_authorized` or `upgrade` in the token module. |
| DeFindex BNSUSDC vault share | [`CBNKCU3HGFKHFOF7JTGXQCNKE3G3DXS5RDBQUKQMIIECYKXPIOUGB2S3`][sx-defindex] | WASM contract, `Traits: sep41`, name `BNSUSDC (DeFindex-Vault-BeansUsdcVault)`, source unavailable | The vault source mints shares on `deposit` and burns them on `withdraw`, has `Manager`, `EmergencyManager`, `RebalanceManager` and fee-receiver roles, a manager-gated `upgrade`, and pauses strategies — not holder balances. No clawback or per-holder freeze. |
| JQQQ | [`CAQYPRTX2E6H6BM57QRBYZWP667FPDYDUHMOQXGXM2HA64KO6AISRFEK`][sx-jqqq] | WASM contract, `Traits: sep41`, name `JQQQ (JanusHendersonNASDAQ-100IndexFund)`, source unavailable | SEP-41 interface observed. [Published research on the Centrifuge deRWA family][redstone-rwa] describes freely transferable tokens with a blocklist; this contract's exact admin surface could not be verified without source. |
| HYB | [`CBNT35PZXQBXKCNKPA6AL6TXHANGLAZMPE35TJUPI2BCO7OGHVKW3WYA`][sx-hyb] | WASM contract, `Traits: sep41`, name `NYLIM US High Yield Bond Fund`, source unavailable | Same Centrifuge/RWA family as JQQQ; only the SEP-41 trait is mechanically verified. |
| Gravity HITZ | [`CBAPZAZNNB4X3VPXV2LYA5RMV7XHXIVREES2GG7R5GUXDZ4R4CKOY4EU`][sx-hitz] | WASM contract, `Traits: sep41`, **source verified** at [`skyhitz/hitz-gravity`][hitz-src] | Exports admin-gated `mint` with a supply cap, `set_admin`, `renounce_admin`, and a 7-day timelocked `propose_upgrade`/`upgrade`. No `clawback`, `freeze` or `set_authorized`; transfer gating is a pool/Event-Horizon rule, not an issuer balance power. |

Two further Centrifuge tokens are listed as live in the issuer's
[`stellar.toml`][centrifuge-toml]: `deJTRSY`
(`CBI7UCH5KGSVQRO5H4SUCZUTZABCITZLRHQQZTWL2TK4RZ72TAR6IHRV`) and `deJAAA`
(`CC64WBDGS6QQP22QTTIACYIXT3WF7BBQEYOQPLTP7GTKYY7PZ74QYGSL`). They are not
counted toward the five above because their StellarExpert pages did not load
at survey time.

The deployed examples show three recurring shapes:

1. **Pool/vault share tokens** (Soroswap LP, DeFindex vault shares). Mint and
   burn exist, but they are side effects of deposit/withdraw and are not
   exposed as a privileged issuer mint. The admin surface, when present,
   controls strategy pausing, roles and upgrades, not holder balances.
2. **RWA/compliance tokens** (Centrifuge deRWA family). SEP-41 transfers plus
   a blocklist or allowlist policy layer is common. [Published research][redstone-rwa]
   describes the Centrifuge deRWA tokens as freely transferable with a
   blocklist, rather than issuer clawback.
3. **Project tokens with real admin mint** (Gravity HITZ). An exported,
   admin-gated `mint` and a timelocked upgrade path exist, and the admin can
   transfer or permanently renounce the role.

## What OpenZeppelin's templates add

The OpenZeppelin Stellar Contracts library is the common starting point for
new contract tokens. Its fungible-token module implements SEP-41 and offers
extensions for `Burnable`, `Capped`, `Mintable`, `Pausable`, `Allowlist` and
`Blocklist`, plus `Ownable` or role-based access control and an upgradeable
utility. The base SEP-41 trait does not include a privileged `mint`; the
`mint` function is an optional extension. There is no clawback in the base
fungible module — a compliance clawback must be written as custom code or
added through a SAC-admin adapter.

So across templates and deployed examples, the frequency is roughly:
`mint` and `burn` common; `pause`/blocklist/allowlist common in regulated
products; `upgrade` common when the project uses the upgradeable utility;
`clawback` uncommon and never implied by SEP-41 itself; per-holder `freeze`
usually implemented as a transfer restriction rather than a balance lock.

## What would be measurable

Given a contract ID and no trust in the deployer, all of the following can be
derived mechanically:

- **Detected interface features.** Read `contractspecv0` from the deployed
  WASM (or call the RPC's contract-spec surface). Entrypoint names such as
  `mint`, `burn`, `clawback`, `freeze`, `set_authorized`, `set_admin`,
  `pause`, `upgrade` and `propose_upgrade`, and their argument types, are
  observable. This is what StellarExpert's `Traits: sep41` and contract
  interface tab are built from.
- **Presence or absence of a named capability.** If no exported entrypoint
  named `clawback` exists, a caller cannot invoke a function by that name
  today. Absence of a name is stronger evidence than presence: a present name
  does not say who may call it or what it does.
- **Contract metadata and source verification.** `contractmeta` can name a
  version and repository; StellarExpert's validation workflow can tie a WASM
  hash to a commit. When that verification succeeds, the source's exported
  functions and auth checks can be read.
- **Upgrade history.** StellarExpert's version count and code-hash history
  show whether the contract has been upgraded and when.
- **On-chain events.** `mint`, `burn`, `clawback` and custom events show what
  an admin has actually done. They are a record of use, not of capability.
- **Storage shape under a known source.** Admin addresses, role keys and
  allowlist/blocklist entries can be located when the source or a storage
  layout is known. SEP-41 does not standardize any of these keys, so without
  source this is not portable.

For Assay, this suggests a future report labeled explicitly as an
*interface-derived* observation (for example "exports an admin-gated `mint`;
source verified") and never folded into `Safety.severity`. It would not be a
consensus fact like a classic issuer flag.

## What cannot be concluded

- **Whether the admin will exercise a capability.** A verified admin `mint`
  does not make a token unsafe, and its absence does not make one safe.
- **Whether the deployed bytecode matches the published source.** If source
  verification is unavailable, an interface probe sees only the WASM. A
  malicious deployer can publish a clean repository and deploy different code.
- **Whether an entrypoint is privilege-gated.** The spec exposes names and
  types, not authorization. `mint` may be admin-only or permissionless.
- **Whether hidden entrypoints exist.** A contract can expose a generic
  dispatcher instead of named functions; a name-based probe would miss it.
  The spec is derived from the deployed WASM, so it lists what is exported,
  but a generic export can still hide behavior behind an argument.
- **Who controls a role or key, or how that key is custodied.** Role
  membership can be read from storage only with source knowledge; key
  compromise is outside the contract entirely.
- **What an upgrade will do.** An `upgrade` entrypoint means the current
  surface is not final. A token with no clawback today can gain one.
- **Semantics of a freeze.** A transfer blocklist, a paused contract and a
  balance lock all deny movement, but an integrator needs to know which
  operation is blocked and whether balances can be restored.
- **Off-chain controls.** KYC, redemption queues and issuer policy live
  outside the contract and are not measurable from WASM.

The honest conclusion is narrow: a SEP-41 token's admin surface can be
*described* mechanically from its exported interface, verified source and
upgrade history, but its safety cannot be derived the way a classic issuer's
capability is. Assay should keep returning unattested for these tokens until a
report can carry that distinction in its type, not only in its prose.

## References

- [SEP-41: Stellar Token Interface][sep41]
- [Stellar token interface documentation][token-interface]
- [Stellar Asset Contract administrative interface][sac]
- [OpenZeppelin Stellar Contracts: fungible token][oz-fungible]
- [OpenZeppelin Stellar Contracts: extensions and access control][oz-contracts]
- [StellarExpert contract metadata for the five surveyed tokens][sx-soroswap]
- [StellarExpert contract-code validation workflow][sx-validation]
- [RedStone: the next phase for RWAs on Stellar (2026)][redstone-rwa]

[sep41]: https://github.com/stellar/stellar-protocol/blob/master/ecosystem/sep-0041.md
[token-interface]: https://developers.stellar.org/docs/tokens/token-interface
[sac]: https://developers.stellar.org/docs/tokens/stellar-asset-contract
[oz-fungible]: https://docs.openzeppelin.com/stellar-contracts/tokens/fungible/fungible
[oz-contracts]: https://developers.stellar.org/docs/tools/openzeppelin-contracts
[sx-soroswap]: https://stellar.expert/explorer/public/contract/CA2TZIB56KYKD46F7IFBF6XPO5TDNK6N2U6BRTGZ5AF4WUSBN6BKZMGF
[sx-defindex]: https://stellar.expert/explorer/public/contract/CBNKCU3HGFKHFOF7JTGXQCNKE3G3DXS5RDBQUKQMIIECYKXPIOUGB2S3
[sx-jqqq]: https://stellar.expert/explorer/public/contract/CAQYPRTX2E6H6BM57QRBYZWP667FPDYDUHMOQXGXM2HA64KO6AISRFEK
[sx-hyb]: https://stellar.expert/explorer/public/contract/CBNT35PZXQBXKCNKPA6AL6TXHANGLAZMPE35TJUPI2BCO7OGHVKW3WYA
[sx-hitz]: https://stellar.expert/explorer/public/contract/CBAPZAZNNB4X3VPXV2LYA5RMV7XHXIVREES2GG7R5GUXDZ4R4CKOY4EU
[hitz-src]: https://github.com/skyhitz/hitz-gravity/tree/2807ed1fecc6f3e58f09d30db34f10931246395f/contracts/gravity-token/src
[centrifuge-toml]: https://centrifuge.io/.well-known/stellar.toml
[sx-validation]: https://stellar.expert/explorer/public/contract/validation
[redstone-rwa]: https://www.redstone.finance/research/next-phase-rwa-stellar-2026
