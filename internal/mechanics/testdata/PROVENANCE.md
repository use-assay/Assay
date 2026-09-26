# Fixture provenance

Captured 2026-08-10 from live public sources.
Each directory is one labelled subject for the eval in docs/eval.md.

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

## Synthetic fixtures

`synthetic-flag-disagreement/` is **not a real capture**. It is derived from
`usdc-revocable-regulated/` (captured 2026-08-10) with one field deliberately
altered: `account.json` reports `auth_clawback_enabled: true` while
`asset.json` reports it false.

No such asset was observed. Horizon's two copies of the issuer flags agreed on
every asset checked live on 2026-09-25, which is precisely why the disagreement
path needs a constructed fixture — it cannot be captured from the network. It
exercises the resolution rule in `reconcileFlags`, and is driven by
`check_capability_disagreement_test.go` rather than by `TestEval`, so the
labelled eval set continues to contain only real captures.
