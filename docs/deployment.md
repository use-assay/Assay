# Deployment

The Assay safety registry is deployed on **Stellar testnet**. It is not on
pubnet, and nothing here should be read as a claim that it is ready to be.

## Live addresses

| | |
| --- | --- |
| **Registry contract** | `CBK4FBIHMDTXCUPE4E3ZDVSFJSCY5FJETTKNIQPN4LFJIKKIBLKIXQ73` |
| Network | Testnet (`Test SDF Network ; September 2015`) |
| Deployed | 2026-08-15 |
| Wasm hash | `c4105b91b3ceae95b5a225c55bc6981b3dcf71d07fdd0ee5c79a21d25edd301b` |
| Source | [`contracts/safety-registry`](../assay-contracts/contracts/safety-registry) |
| Admin / attester | `GALIEUOBDLTFJHTVH5E3MT2BMDTQ3PKMX2U6BRXVKLEB7ARFORFNNMVY` |
| Built with | `stellar` CLI 27.1.0, `soroban-sdk` 27.0.5 |

The worked integration example from [integrating.md](integrating.md) is
deployed alongside it, wired to the registry above:

| | |
| --- | --- |
| **Example gate contract** | `CANO57JRGTATHGLM26TWYPIXERSPVI5R52H33K7ZUJGGOEOVVZA44W3U` |
| Wasm hash | `d1683a1ed24d7645168f5c212ad3f7d8979331b41e53ffa9eaa7a60babe32ebe` |
| Source | [`contracts/example-gate`](../assay-contracts/contracts/example-gate) |

### Deployment transactions

| Step | Transaction |
| --- | --- |
| Upload registry wasm | `2c6b1e695fbaafa2c5134c6b57a0654371a8d9d7e90d70f3f85404a7709d53f1` |
| Deploy registry | `ff11e24ff1d9ccd4aeea4dc258ce17d49a7b0d0c529a326f7eda9d6eff2f1c9c` |
| `init(admin)` | `14082d78211ad494406c14d6f0bd2993a88008cb4091457a7685b3794d20ee09` |
| Upload gate wasm | `2dc8387ff2a4ef2a24f788de627f566dfc6d60d96806f7ebffb529324056233e` |
| Deploy gate | `a6c4f642af24f32ec116a8a8918153cacafb86f9ac33c73dfb942dce73d5897f` |

Any of these can be read at
`https://stellar.expert/explorer/testnet/tx/<hash>`.

## Attested assets

Seven mainnet assets, spanning the severity range. Every number below was
produced by `assay attestation` from a live scan and submitted unmodified by
`make attest`; none was typed by hand. The full scan record, including the six
further assets scanned but not attested, is in
[attestation-run.md](attestation-run.md).

| Asset | Severity | Flags | Mechanics | Attest transaction |
| --- | --- | --- | --- | --- |
| `AQUA` | 0 clear | `0` | — | `1b6bafc1226570b2415299f5531256716f4d8dc489a9784fcd6ea347d0f63f5f` |
| `USDC` | 2 medium | `18` | `auth_revocable`, `domain_unverified` | `9869957e510ac6cb9ba4e5ffaff66ad0286e420e824efc0095d9e578be37564e` |
| `USDZ` | 3 high | `6` | `auth_revocable`, `auth_clawback_enabled` | `069d519fa20c472bbc3756291307fb30719219903c1e62a27b4967c0b07ecfd6` |
| `BERKSHIRE` | 4 critical | `54` | `auth_revocable`, `auth_clawback_enabled`, `domain_unverified`, `blocklisted` | `322f4bb5fcc84cea9231b7d12e0a964a304a7bd4ea78f97991ca335426e5e2ff` |
| `ARST` | 0 clear | `0` | — | `f497b91ab84340bcb7418940e620d080c8566e0cdf27346c8d74e165b5d66506` |
| `USDGLO` | 3 high | `6` | `auth_revocable`, `auth_clawback_enabled` | `6016ed7d908cd20237f114b0cb6778886027ddc8336952acb77ddbeff16caf42` |
| `DOGE` | 4 critical | `48` | `domain_unverified`, `blocklisted` | `ac1a89a64159e6ac2e9ed61bd67a79cd1584287cf57d19f9dc188d80e81298a6` |

Issuers, SAC addresses, and the evidence hash each attestation commits to:

| Asset | Issuer | Testnet SAC address | `evidence_hash` |
| --- | --- | --- | --- |
| `AQUA` | `GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA` | `CDJF2JQINO7WRFXB2AAHLONFDPPI4M3W2UM5THGQQ7JMJDIEJYC4CMPG` | `688453bd…61a4a9` |
| `USDC` | `GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN` | `CA2E53VHFZ6YSWQIEIPBXJQGT6VW3VKWWZO555XKRQXYJ63GEBJJGHY7` | `e7fc765d…d790bd` |
| `USDZ` | `GAKTLPC4ZV37SSCITQ5IS5AQ4WPF4CF4VZJQPPAROSGXMYOATF5U6XPR` | `CAOM5NKBTSGEXTZZKH3STWSFWURMODC3TZ4NS2THN7W5YUDFK3IOHIHU` | `ca9b13a6…65fb4d` |
| `BERKSHIRE` | `GA22QHSHQEHDJS2ZOINSC77XPPQ24G5EFRJGVEIZLKC5FAW3PQ5XNSDQ` | `CALAZXOC32XF2DFRKDOFQK7XQETZZTUK7C2OKC6YKPX6KY2YWU2YSPQW` | `dc2bbf08…171a0d` |
| `ARST` | `GCSAZVWXZKWS4XS223M5F54H2B6XPIIXZZGP7KEAIU6YSL5HDRGCI3DG` | `CBARCMJYRRNSYCWCR3EU2PEHAHWHBCQSMIKQIUSDWR3BK7CBCP622Q2R` | `82a6103f…fccacb` |
| `USDGLO` | `GBBS25EGYQPGEZCGCFBKG4OAGFXU6DSOQBGTHELLJT3HZXZJ34HWS6XV` | `CDGBOKCE25PVUKFWST2EEK52NHRS5WQ7TN26DFJYCEQNZNROQRSPIBQA` | `69b8c4f8…7c27a5` |
| `DOGE` | `GA22IDJNHUMC3XKUCCBFNTQIJOUBWINC5GCXHLJ2V6KZ3OWAXCULNQ7P` | `CDUV37BUTYKKWNGECZZNRYMM7JIQYYWAI7L2TPTXWQAEMIPG4SXRBRPD` | `396c9f7c…91647e` |

`USDZ` is the case the [severity model](severity-model.md) exists to handle: a
confiscation-capable issuer with a *verified* domain. It scores `high` on
capability anyway, because a verified domain does not weaken clawback — it only
names who holds it. `BERKSHIRE` is the escalation case: capability alone puts it
at `high`, and StellarExpert's blocklisting of `nasdaq.finance` raises it to
`critical`. Reputation moved it up; nothing in the pipeline can move an asset
down.

`DOGE` is the same escalation shown at its extreme, and is the more instructive
of the two: base severity `0`, final severity `4`. Its issuer holds no
authorization flags at all, so capability analysis honestly returns `clear` for
an asset the curated directory tags `malicious`. Everything separating it from
a harmless asset lives on the reputation axis.

### The assets are mainnet; the attestations are testnet

These assets live on pubnet. Their **testnet** SAC addresses are what the
registry is keyed on here, because a contract ID is derived from the network
passphrase and so differs per network. Scanning reads pubnet, where the assets
actually are; attesting writes to testnet, where the contract actually is. A
pubnet deployment would key the same assets under their pubnet SAC addresses.

## Reproducing the round trip

Scanner output and on-chain state, read back after the fact:

```
$ ./assay attestation -raw USDZ-GAKTLPC4ZV37SSCITQ5IS5AQ4WPF4CF4VZJQPPAROSGXMYOATF5U6XPR
3	6	ca9b13a66f3a0a4b43d66dea29a505658447e08eee200d2bdb5e66aa1065fb4d

$ make read ASSET=USDZ-GAKTLPC4ZV37SSCITQ5IS5AQ4WPF4CF4VZJQPPAROSGXMYOATF5U6XPR
{"attested_at":1786771651,"evidence_hash":"ca9b13a66f3a0a4b43d66dea29a505658447e08eee200d2bdb5e66aa1065fb4d","flags":6,"severity":3}
```

The severity, the bitset, and the evidence hash match, and the hash was
recomputed from a *fresh* scan after the write — which is the point of leaving
retrieval timestamps out of the preimage. A verifier who re-scans an asset whose
sources have not changed reproduces the on-chain hash exactly. If it does not
reproduce, either the asset changed or the attestation is not what it claims.

`attested_at` is the ledger timestamp of the write: `1786771651` is
2026-08-15T05:27:31Z.

## Fail-closed, verified live

Checked against the deployed contract before any attestation existed, and again
afterwards using `native` XLM as a control asset that is deliberately never
attested (SAC `CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC`):

| Call | Result |
| --- | --- |
| `get_safety(unattested)` | `null` |
| `is_safe(unattested, max_severity=4, max_age_secs=0)` | `false` |
| `is_safe(unattested, max_severity=0, max_age_secs=0)` | `false` |

The middle row is the one that matters: severity 4 is the maximum and age 0
disables the freshness check, so those are the most permissive arguments the
gate accepts. It still returns `false`, because there is nothing to admit. An
unknown asset is unknown, not safe.

`is_safe` across the attested set, read live:

| Asset | `max_severity=0` | `max_severity=2` |
| --- | --- | --- |
| `AQUA` (0) | `true` | `true` |
| `USDC` (2) | `false` | `true` |
| `USDZ` (3) | `false` | `false` |
| `BERKSHIRE` (4) | `false` | `false` |
| unattested | `false` | `false` |

And through the deployed example gate, which is a real cross-contract call
rather than a direct read — `would_admit` returns `true` only for `AQUA`, and a
submitted `deposit` reverts with the contract error naming the reason:

| Asset | `would_admit` | `deposit` |
| --- | --- | --- |
| `AQUA` | `true` | succeeded, balance credited (`7dde01b27c1c60f2c5f94ce453b652ca8fcd685fe8e1950ae6a4700b100679a0`) |
| `USDZ` | `false` | reverts `Error(Contract, #3)` — `IssuerCanTakeIt` |
| unattested | `false` | reverts `Error(Contract, #1)` — `NotAttested` |

The gate refuses `USDC` too, at severity 2, because it reads the
`auth_revocable` bit rather than the severity number.

It also **wrongly admits `DOGE`** for the same reason: the example masks
capability bits only, and DOGE's severity `4` comes entirely from reputation,
which sets no capability bit. A correct gate reads both axes. See
[integrating.md](integrating.md) for the corrected pattern and
[#26](https://github.com/use-assay/Assay/issues/26) for the contract fix.

## Redeploying

```sh
make build-contract          # stellar contract build -> assay-contracts/out/
make deploy-testnet          # upload + deploy, prints the new contract ID
stellar contract invoke --id <NEW_ID> --source-account assay-attester \
  --network testnet -- init --admin $(stellar keys address assay-attester)
```

Then update `CONTRACT_ID` in the [Makefile](../Makefile) and the addresses in
this file. `init` is single-shot; a second call fails with
`AlreadyInitialized`.

Attesting and reading, once deployed:

```sh
make attest ASSET=AQUA-GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA
make read   ASSET=AQUA-GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA
```

`make attest` scans the asset live, derives the arguments with
`assay attestation`, and submits them. There is no path through it that lets a
hand-written severity reach the contract.

## What this deployment is not

- **Not mainnet, and not a candidate for it.** The list below is why.
- **One key can write anything.** The admin is a single ed25519 account whose
  seed lives on one machine. Anyone holding it can attest any severity for any
  asset. A real deployment wants a threshold of independent attesters.
- **7 attested assets.** Everything else on the network reads as `None`. That is the
  correct answer — unknown, not safe — but it means the registry is not useful
  as a general lookup yet.
- **No re-attestation schedule.** These attestations are as fresh as the
  `attested_at` in the table and nothing is refreshing them. A caller must pass
  a `max_age_secs` it is actually willing to accept rather than trusting that
  someone is keeping the registry current.
- **Testnet data is not durable.** Testnet is periodically reset, and Soroban
  persistent entries expire if their TTL is not extended. Both will take these
  attestations away without warning.
