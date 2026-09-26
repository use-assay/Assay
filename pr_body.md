- closes #3
- closes #4
- closes #65
- closes #68

### Changes Made:
- **SEP-1 Checks**: Implemented `check_issuer_home_domain` to flag issuers missing a `home_domain` as unattributable. Added `check_reciprocity_mismatch` to strictly compare `home_domain` against the curated directory domain.
- **Fixtures**: Added comprehensive historical observation fixtures in `src/fixtures/` for temporal testing.
- **Documentation**: Wrote `temporal_trust_model.md` to officially document the temporal trust and decay models.
