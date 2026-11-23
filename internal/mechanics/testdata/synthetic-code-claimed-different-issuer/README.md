# synthetic-code-claimed-different-issuer

Synthetic fixture for issue #379.

The issuer is the real AQUA account (GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA)
with no authorization flags set (capability=clear). The account's
`home_domain` is set to the synthetic domain `impersonation.example`.

The `stellar.toml` at `impersonation.example` claims the AQUA code but under a
**different issuer** (GDSTRSHXHGJ7ZIVRBXEYE5Q74XUVCUSEKEBR7UCHEUUEK72N7I7KJ6JH —
the SHX issuer). This is the impersonation shape the reciprocity rule exists to
catch: `Claims(code, issuer)` requires both code AND issuer to match, so a
toml that lists AQUA under a different issuer does not satisfy the
round-trip.

The `directory.json` is set to agree with `home_domain` (`domain:
"impersonation.example"`) so the domain-vs-directory check does not fire
first, and the test reaches the `Claims()` comparison. The expected result is
accountability=unverified with mechanic `domain_unverified`.
