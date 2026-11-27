# Fixture provenance

Captured 2026-08-10 from live public sources.
Each directory is one labelled subject for the eval in docs/eval.md.
This file is the human index; each subject's `metadata.json` is the
machine-readable record the eval loads — per-file source URL, capture date,
and the expected verdict.

| file | source URL |
| --- | --- |
| `aqua-clear-verified/asset.json` | https://horizon.stellar.org/assets?asset_code=AQUA&asset_issuer=GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA |
| `aqua-clear-verified/account.json` | https://horizon.stellar.org/accounts/GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA |
| `aqua-clear-verified/stellar.toml` | https://aqua.network/.well-known/stellar.toml |
| `aqua-clear-verified/blocked.json` | https://api.stellar.expert/explorer/directory/blocked-domains/aqua.network |
| `aqua-clear-verified/directory.json` | https://api.stellar.expert/explorer/directory/GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA |
| `shx-clear-flagslocked/asset.json` | https://horizon.stellar.org/assets?asset_code=SHX&asset_issuer=GDSTRSHXHGJ7ZIVRBXEYE5Q74XUVCUSEKEBR7UCHEUUEK72N7I7KJ6JH |
| `shx-clear-flagslocked/account.json` | https://horizon.stellar.org/accounts/GDSTRSHXHGJ7ZIVRBXEYE5Q74XUVCUSEKEBR7UCHEUUEK72N7I7KJ6JH |
| `shx-clear-flagslocked/stellar.toml` | https://stronghold.co/.well-known/stellar.toml |
| `shx-clear-flagslocked/blocked.json` | https://api.stellar.expert/explorer/directory/blocked-domains/stronghold.co |
| `shx-clear-flagslocked/directory.json` | https://api.stellar.expert/explorer/directory/GDSTRSHXHGJ7ZIVRBXEYE5Q74XUVCUSEKEBR7UCHEUUEK72N7I7KJ6JH |
| `usdc-revocable-regulated/asset.json` | https://horizon.stellar.org/assets?asset_code=USDC&asset_issuer=GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN |
| `usdc-revocable-regulated/account.json` | https://horizon.stellar.org/accounts/GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN |
| `usdc-revocable-regulated/stellar.toml.status` | https://circle.com/.well-known/stellar.toml (HTTP 404) |
| `usdc-revocable-regulated/blocked.json` | https://api.stellar.expert/explorer/directory/blocked-domains/circle.com |
| `usdc-revocable-regulated/directory.json` | https://api.stellar.expert/explorer/directory/GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN |
| `usdc-revocable-regulated/stellar-expert-asset.json` | https://api.stellar.expert/explorer/public/asset/USDC-GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN (captured 2026-09-25) |
| `berkshire-clawback-scam/asset.json` | https://horizon.stellar.org/assets?asset_code=BERKSHIRE&asset_issuer=GA22QHSHQEHDJS2ZOINSC77XPPQ24G5EFRJGVEIZLKC5FAW3PQ5XNSDQ |
| `berkshire-clawback-scam/account.json` | https://horizon.stellar.org/accounts/GA22QHSHQEHDJS2ZOINSC77XPPQ24G5EFRJGVEIZLKC5FAW3PQ5XNSDQ |
| `berkshire-clawback-scam/stellar.toml.status` | https://nasdaq.finance/.well-known/stellar.toml (HTTP 000) |
| `berkshire-clawback-scam/blocked.json` | https://api.stellar.expert/explorer/directory/blocked-domains/nasdaq.finance |
| `berkshire-clawback-scam/directory.json` | https://api.stellar.expert/explorer/directory/GA22QHSHQEHDJS2ZOINSC77XPPQ24G5EFRJGVEIZLKC5FAW3PQ5XNSDQ |
| `doge-noflags-scam/asset.json` | https://horizon.stellar.org/assets?asset_code=DOGE&asset_issuer=GA22IDJNHUMC3XKUCCBFNTQIJOUBWINC5GCXHLJ2V6KZ3OWAXCULNQ7P |
| `doge-noflags-scam/account.json` | https://horizon.stellar.org/accounts/GA22IDJNHUMC3XKUCCBFNTQIJOUBWINC5GCXHLJ2V6KZ3OWAXCULNQ7P |
| `doge-noflags-scam/stellar.toml.status` | https://darkpool.digital/.well-known/stellar.toml (HTTP 000) |
| `doge-noflags-scam/blocked.json` | https://api.stellar.expert/explorer/directory/blocked-domains/darkpool.digital |
| `doge-noflags-scam/directory.json` | https://api.stellar.expert/explorer/directory/GA22IDJNHUMC3XKUCCBFNTQIJOUBWINC5GCXHLJ2V6KZ3OWAXCULNQ7P |
| `doge-reputation-outage/asset.json` | https://horizon.stellar.org/assets?asset_code=DOGE&asset_issuer=GA22IDJNHUMC3XKUCCBFNTQIJOUBWINC5GCXHLJ2V6KZ3OWAXCULNQ7P |
| `doge-reputation-outage/account.json` | https://horizon.stellar.org/accounts/GA22IDJNHUMC3XKUCCBFNTQIJOUBWINC5GCXHLJ2V6KZ3OWAXCULNQ7P |
| `doge-reputation-outage/stellar.toml.status` | https://darkpool.digital/.well-known/stellar.toml (HTTP 000) |
| `doge-reputation-outage/blocked.json` | https://api.stellar.expert/explorer/directory/blocked-domains/darkpool.digital |
| `doge-reputation-outage/directory.err` | https://api.stellar.expert/explorer/directory/GA22IDJNHUMC3XKUCCBFNTQIJOUBWINC5GCXHLJ2V6KZ3OWAXCULNQ7P (HTTP 429) |
| `velo-no-home-domain/asset.json` | https://horizon.stellar.org/assets?asset_code=VELO&asset_issuer=GDM4RQUQQUVSKQA7S6EM7XBZP3FCGH4Q7CL6TABQ7B2BEJ5ERARM2M5M (captured 2026-09-28) |
| `velo-no-home-domain/account.json` | https://horizon.stellar.org/accounts/GDM4RQUQQUVSKQA7S6EM7XBZP3FCGH4Q7CL6TABQ7B2BEJ5ERARM2M5M (captured 2026-09-28) |
| `velo-no-home-domain/directory.json` | https://api.stellar.expert/explorer/directory/GDM4RQUQQUVSKQA7S6EM7XBZP3FCGH4Q7CL6TABQ7B2BEJ5ERARM2M5M (captured 2026-09-28) |
| `xrp-clear-unlocked/asset.json` | https://horizon.stellar.org/assets?asset_code=XRP&asset_issuer=GBXRPL45NPHCVMFFAYZVUVFFVKSIZ362ZXFP7I2ETNQ3QKZMFLPRDTD5 (captured 2026-09-25) |
| `xrp-clear-unlocked/account.json` | https://horizon.stellar.org/accounts/GBXRPL45NPHCVMFFAYZVUVFFVKSIZ362ZXFP7I2ETNQ3QKZMFLPRDTD5 (captured 2026-09-25) |
| `xrp-clear-unlocked/stellar.toml` | https://fchain.io/.well-known/stellar.toml (captured 2026-09-25) |
| `xrp-clear-unlocked/blocked.json` | https://api.stellar.expert/explorer/directory/blocked-domains/fchain.io (captured 2026-09-25) |
| `xrp-clear-unlocked/directory.json` | https://api.stellar.expert/explorer/directory/GBXRPL45NPHCVMFFAYZVUVFFVKSIZ362ZXFP7I2ETNQ3QKZMFLPRDTD5 (captured 2026-09-25) |
| `doge-disagreeing-sources/asset.json` | https://horizon.stellar.org/assets?asset_code=DOGE&asset_issuer=GA22IDJNHUMC3XKUCCBFNTQIJOUBWINC5GCXHLJ2V6KZ3OWAXCULNQ7P (captured 2026-09-22) |
| `doge-disagreeing-sources/account.json` | https://horizon.stellar.org/accounts/GA22IDJNHUMC3XKUCCBFNTQIJOUBWINC5GCXHLJ2V6KZ3OWAXCULNQ7P (captured 2026-09-22) |
| `doge-disagreeing-sources/stellar.toml.status` | https://darkpool.digital/.well-known/stellar.toml (captured 2026-09-22, HTTP 000) |
| `doge-disagreeing-sources/blocked.json` | https://api.stellar.expert/explorer/directory/blocked-domains/darkpool.digital (captured 2026-09-22, returns `blocked: false`) |
| `doge-disagreeing-sources/directory.json` | https://api.stellar.expert/explorer/directory/GA22IDJNHUMC3XKUCCBFNTQIJOUBWINC5GCXHLJ2V6KZ3OWAXCULNQ7P (captured 2026-09-22, tags `malicious`, `unsafe`) |
| `usdz-clawback-regulated/asset.json` | https://horizon.stellar.org/assets?asset_code=USDZ&asset_issuer=GAKTLPC4ZV37SSCITQ5IS5AQ4WPF4CF4VZJQPPAROSGXMYOATF5U6XPR (captured 2026-09-27) |
| `usdz-clawback-regulated/account.json` | https://horizon.stellar.org/accounts/GAKTLPC4ZV37SSCITQ5IS5AQ4WPF4CF4VZJQPPAROSGXMYOATF5U6XPR (captured 2026-09-27) |
| `usdz-clawback-regulated/stellar.toml` | https://zeam.money/.well-known/stellar.toml (captured 2026-09-27; abridged to the USDZ CURRENCIES entry, the DOCUMENTATION block, and the header — the full file lists many other assets and validators irrelevant to this subject) |
| `usdz-clawback-regulated/blocked.json` | https://api.stellar.expert/explorer/directory/blocked-domains/zeam.money (captured 2026-09-27) |
| `usdz-clawback-regulated/directory.json` | https://api.stellar.expert/explorer/directory/GAKTLPC4ZV37SSCITQ5IS5AQ4WPF4CF4VZJQPPAROSGXMYOATF5U6XPR (captured 2026-09-27) |
| `synthetic-reputation-outage/*` | Not captured. Payload files are copies of `aqua-clear-verified` (2026-08-10); `blocked.err` is a constructed error marker (HTTP 429) added 2026-09-27 — see that directory's README. |
| `synthetic-redirect-off-host/*` | Not captured. Payload files based on `aqua-clear-verified` (2026-08-10); `stellar.toml.err` records the cross-host redirect refusal error from redirect.example to other.example (added 2026-10-16, #380) — see that directory's README. |
| `synthetic-auth-required-gating/asset.json` | https://horizon.stellar.org/assets?asset_code=AQUA&asset_issuer=GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA — synthetic: derived from aqua-clear-verified with auth_required=true (#376) |
| `synthetic-auth-required-gating/account.json` | https://horizon.stellar.org/accounts/GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA — synthetic: auth_required=true (#376) |
| `synthetic-auth-required-gating/stellar.toml` | https://aqua.network/.well-known/stellar.toml (copied from aqua-clear-verified, 2026-08-10, #376) |
| `synthetic-auth-required-gating/blocked.json` | https://api.stellar.expert/explorer/directory/blocked-domains/aqua.network (copied from aqua-clear-verified, 2026-08-10, #376) |
| `synthetic-auth-required-gating/directory.json` | https://api.stellar.expert/explorer/directory/GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA (copied from aqua-clear-verified, 2026-08-10, #376) |
| `synthetic-code-claimed-different-issuer/asset.json` | https://horizon.stellar.org/assets?asset_code=AQUA&asset_issuer=GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA — synthetic: home_domain set to impersonation.example (#379) |
| `synthetic-code-claimed-different-issuer/account.json` | https://horizon.stellar.org/accounts/GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA — synthetic: home_domain impersonation.example (#379) |
| `synthetic-code-claimed-different-issuer/stellar.toml` | https://impersonation.example/.well-known/stellar.toml — constructed: claims AQUA code under wrong issuer GDSTRSHXHGJ7ZIVRBXEYE5Q74XUVCUSEKEBR7UCHEUUEK72N7I7KJ6JH (2026-11-22, #379) |
| `synthetic-code-claimed-different-issuer/blocked.json` | https://api.stellar.expert/explorer/directory/blocked-domains/impersonation.example — constructed synthetic payload (2026-11-22, #379) |
| `synthetic-code-claimed-different-issuer/directory.json` | https://api.stellar.expert/explorer/directory/GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA — constructed: domain set to impersonation.example so it agrees with home_domain (2026-11-22, #379) |
| `zzz-signer-locked/asset.json` | https://horizon.stellar.org/assets?asset_code=ZZZXLM&asset_issuer=GA2TD277OP77MSXVKN4JNJRJQBHXCQL5X3JJMPQ2CDZF462746FVWXZ4 (captured 2026-11-23, #359) |
| `zzz-signer-locked/account.json` | https://horizon.stellar.org/accounts/GA2TD277OP77MSXVKN4JNJRJQBHXCQL5X3JJMPQ2CDZF462746FVWXZ4 (captured 2026-11-23; signers show master weight 0 and no other signer, thresholds 1/1/1, #359) |
| `zzz-signer-locked/stellar.toml` | https://zzztokens.netlify.app/.well-known/stellar.toml (captured 2026-11-23, in full) |
| `zzz-signer-locked/blocked.json` | https://api.stellar.expert/explorer/directory/blocked-domains/zzztokens.netlify.app (captured 2026-11-23, returns `blocked: false`) |
| `usdc-signers-hold-power/asset.json` | https://horizon.stellar.org/assets?asset_code=USDC&asset_issuer=GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN (copied from usdc-revocable-regulated, 2026-08-10, #359) |
| `usdc-signers-hold-power/account.json` | https://horizon.stellar.org/accounts/GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN (captured 2026-11-23; signers show master weight 0 with four weight-1 signers and thresholds 2/2/2, #359) |
| `usdc-signers-hold-power/stellar.toml.status` | https://circle.com/.well-known/stellar.toml (copied from usdc-revocable-regulated, 2026-08-10, HTTP 404) |
| `usdc-signers-hold-power/blocked.json` | https://api.stellar.expert/explorer/directory/blocked-domains/circle.com (copied from usdc-revocable-regulated, 2026-08-10) |
| `usdc-signers-hold-power/directory.json` | https://api.stellar.expert/explorer/directory/GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN (copied from usdc-revocable-regulated, 2026-08-10) |
| `usdc-signers-hold-power/stellar-expert-asset.json` | https://api.stellar.expert/explorer/public/asset/USDC-GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN (copied from usdc-revocable-regulated, 2026-09-25) |
| `synthetic-sac-admin-moved/asset.json` | https://horizon-testnet.stellar.org/assets?asset_code=zUSDC&asset_issuer=GAA3S6XLOKFX3SGDQ3VGLLXMCFMVB7E6WYNGHRGIRD62AEJ73ASPQ4KX (captured 2026-11-23, #356) |
| `synthetic-sac-admin-moved/account.json` | https://horizon-testnet.stellar.org/accounts/GAA3S6XLOKFX3SGDQ3VGLLXMCFMVB7E6WYNGHRGIRD62AEJ73ASPQ4KX (captured 2026-11-23, #356) |
| `synthetic-sac-admin-moved/sac-admin.json` | https://stellar.expert/explorer/testnet/contract/CAIUOHVZ77RSCDBNWR3BCZPTWHPUXQRTQXSW4VE3HGC2M5PRPJNSFBRU — read with getLedgerEntries from https://soroban-testnet.stellar.org (2026-11-23, #356); the raw entry XDR is committed at `internal/sacadmin/testdata/moved-admin.b64` |
| `synthetic-sac-admin-issuer/asset.json` | Constructed from the live testnet issuer account GAIFQB42YWW6IRWMT7WFFZUB22HJ2SMBZ6SDMVEXALGE6XK5GGB3PPHJ (2026-11-23, #356): ASSAYPRB has no issued trustlines, so Horizon's /assets list has no record for it |
| `synthetic-sac-admin-issuer/account.json` | https://horizon-testnet.stellar.org/accounts/GAIFQB42YWW6IRWMT7WFFZUB22HJ2SMBZ6SDMVEXALGE6XK5GGB3PPHJ (captured 2026-11-23, #356) |
| `synthetic-sac-admin-issuer/sac-admin.json` | https://stellar.expert/explorer/testnet/contract/CAQRN7WMPPS2VBQQVIUQXGRVYIBXPLT65KIQAI52FJCFFZQ6SG4PP7R4 — read with getLedgerEntries from https://soroban-testnet.stellar.org (2026-11-23, #356); deployed by the issuer in transaction ce28eb7dbad21005faff17f39288479c6602d4c3e88f5f385ed1facfb7cbc5e1, and `stellar contract invoke --id CAQRN… -- admin` returned the issuer. The raw entry XDR is committed at `internal/sacadmin/testdata/issuer-admin.b64` |

`doge-reputation-outage` is the same real asset as `doge-noflags-scam`, captured
for the case where a source failed. Its `directory.err` is a deliberate marker,
not a captured body: it records that the directory was asked and returned an
error at capture time, so the fixture can express the outage from issue #23
(see docs/eval.md, "Fixture convention"). The payload files around it are the
same live captures as the `doge-noflags-scam` rows above.

### Independent verification for `usdz-clawback-regulated`

The issue asked for a legitimate clawback-enabled asset and warned against
assuming an issuer is legitimate because it looks institutional. The evidence
this fixture rests on:

- The `zeam.money` stellar.toml lists USDZ under `[[CURRENCIES]]` with
  `issuer = "GAKTLPC4ZV37SSCITQ5IS5AQ4WPF4CF4VZJQPPAROSGXMYOATF5U6XPR"`,
  matching the account's `home_domain`. That is the reciprocal SEP-1 claim.
- The `[DOCUMENTATION]` block names `ZEAM LIMITED` with
  `ORG_LICENSING_AUTHORITY = "Financial Sector Conduct Authority"` and a
  license-number URL. FSCA is South Africa's regulator.
- StellarExpert's directory tags the address as `issuer` (not `malicious`),
  and the blocked-domains endpoint returns `blocked: false` for `zeam.money`.
- The issuer's flags on Horizon are `auth_revocable: true` and
  `auth_clawback_enabled: true`. Both were re-verified live at capture time,
  not assumed.

The asset is also attested on testnet — see `docs/deployment.md` — so this
fixture closes the gap `docs/eval.md#coverage-gaps` flagged.

### Why `velo-no-home-domain` has no toml and no blocked.json

This is the subject issue #3 asks for: an issuer whose account carries no
`home_domain` at all. Verified live on 2026-09-28 that Horizon **omits the
field entirely** for this account rather than returning `""`, so
`account.json` deliberately has no `home_domain` key — the empty-home-domain
case, as the ledger actually serves it.

With no advertised domain there is nothing to fetch: `internal/scan/scan.go`
asks for neither the stellar.toml nor the blocked-domain lookup when
`home_domain` is empty, so those two files correctly do not exist here. The
directory entry is still captured, because the directory is queried by issuer
address regardless.
