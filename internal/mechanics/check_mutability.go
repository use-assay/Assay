package mechanics

import (
	"context"
	"fmt"
	"strings"

	"github.com/use-assay/assay/internal/horizon"
)

// MutabilityCheck reports whether the issuer's authorization flag set can still
// change. It is modelled on DomainCheck: it reports a first-class finding and
// never sets a severity.
//
// auth_immutable is deliberately not a severity level. It is not a power over
// holders; it fixes whether the power set can change, and which direction that
// cuts depends entirely on what is already set. So it cannot be carried by a
// number, and it is reported here instead:
//
//   - Locked with no dangerous flags — a durable safety property. Clawback and
//     freeze can never be added.
//   - Locked with freeze or clawback — permanence. The power can never be given
//     up.
//   - Locked by signers — same durability, from the account's signing
//     configuration rather than its flags. A master key weight of 0, no other
//     signer with weight, and a non-zero medium threshold means no operation
//     can be authorized, so no flag can be changed even though
//     auth_immutable is unset (#359).
//   - Not locked — the issuer may add freeze or confiscation later. Under
//     CAP-0035 that would not reach trustlines that already exist, but it would
//     apply to any trustline opened after the change.
//
// The not-locked case is the one a prospective holder wants surfaced, because
// it is the one where today's clear is not a durable answer. It is stated
// rather than scored, because a single severity number cannot carry a
// conditional.
//
// Locked by signers deliberately does not set MechFlagsLocked: that bit is the
// ledger's record that auth_immutable is set, and this check must not make the
// report claim a flag the issuer does not hold. The state is carried by the
// reasoning, exactly as the other mutability states are.
//
// Flag source: this check reads the reconciled flag set the capability check
// derives severity from — reconcileFlags resolves Horizon's two copies of the
// issuer flags (the /assets record and the /accounts record), counting a power
// held when either copy reports it and auth_immutable as set only when both
// copies agree. Reading that same set here keeps this finding consistent with
// the severity next to it, and means a lock is never claimed while a source
// contradicts it: asserting a lock one copy disputes would hand the reader a
// reassurance the evidence does not support.
//
// It emits no Evidence. The flag read is already attributed by the capability
// finding, and the evidence_hash preimage is a sorted multiset: emitting a
// second copy of the same claim — or a new claim about the same read — would
// change the hash of every attestation whose underlying evidence did not
// change, which the encoding is explicitly designed to prevent.
type MutabilityCheck struct{}

// ID implements Check.
func (MutabilityCheck) ID() string { return "mutability" }

// Describe implements Check.
func (MutabilityCheck) Describe() string {
	return "Reports whether the issuer's flag set can still change: locked by " +
		"auth_immutable with no dangerous flags (a durable safety property), locked " +
		"with freeze or clawback (the power is permanent), locked by the account's " +
		"signer configuration (master key weight 0, no other signers), or unlocked " +
		"(the issuer may add freeze or confiscation later — adding auth_revocable " +
		"reaches existing trustlines at once, while adding auth_clawback_enabled " +
		"under CAP-0035 applies only to trustlines opened after the change). Never " +
		"sets severity: neither auth_immutable nor a signer lock is a power over " +
		"holders."
}

// Inputs implements Check.
//
// The flags are read through reconcileFlags, which compares the asset record's
// copy against the issuer account's, so both are inputs.
func (MutabilityCheck) Inputs() []string {
	return []string{"Stat", "Issuer"}
}

// Run implements Check.
func (c MutabilityCheck) Run(_ context.Context, s *Subject) (Finding, error) {
	f := Finding{
		Check:    c.ID(),
		Title:    "Issuer flag mutability",
		Severity: Clear, // mutability is never severity
		Evidence: []Evidence{},
	}

	if s.Stat == nil {
		f.Undetermined = true
		f.UndeterminedSource = "horizon"
		f.Reasoning = "The asset was not found on the ledger, so no issuer flags " +
			"could be read. Whether the flag set can still change is unknown, " +
			"which is not the same as unlocked and not the same as safe."
		return f, nil
	}

	flags, _ := reconcileFlags(s)
	if flags.AuthImmutable {
		f.Mechanics = MechFlagsLocked
	}

	powers := holderPowers(flags)
	signerLocked, signingKnown := signerLockState(s)
	switch {
	case flags.AuthImmutable && len(powers) == 0:
		f.Reasoning = "The issuer's flag set is locked (auth_immutable is set) and " +
			"the issuer holds no freeze or confiscation power. That is a durable " +
			"safety property: because the flag set can never change again, " +
			"auth_revocable and auth_clawback_enabled can never be added later, so " +
			"this asset is clear for a trustline opened now and stays clear for one " +
			"opened later. This does not move severity; it reports the durability of " +
			"the verdict the capability check already reached."
	case flags.AuthImmutable:
		f.Reasoning = fmt.Sprintf(
			"The issuer's flag set is locked (auth_immutable is set) while the issuer "+
				"can %s. The lock makes the power permanent: no flag in this set can be "+
				"given up, because auth_immutable itself cannot be cleared once set. "+
				"Locking does not make the power safer — it removes any future in which "+
				"the issuer relinquishes it. Severity is unchanged; this is a report "+
				"about durability, not a new capability.",
			joinPowers(powers))
	case signerLocked:
		masterWeight, thresholds, _ := s.Issuer.MasterKeyWeight()
		var b strings.Builder
		_, _ = fmt.Fprintf(&b, "The issuer account is locked by its signer configuration: "+
			"its master key weight is %d, it has no other signer with weight, and its "+
			"medium threshold is %d. No transaction can be authorized, so the flag set "+
			"can never change. ", masterWeight, thresholds.Med)
		if len(powers) == 0 {
			b.WriteString("The issuer holds no freeze or confiscation power today, and none " +
				"can ever be added. That is the same durable safety property as an " +
				"auth_immutable lock, reached without auth_immutable being set. ")
		} else {
			_, _ = fmt.Fprintf(&b, "The issuer can %s today, and that power is permanent: "+
				"no signer can ever submit the operation that would give it up. ", joinPowers(powers))
		}
		b.WriteString("Severity is unchanged; this reports why the verdict is durable, not " +
			"a new capability. Locked-by-signers does not set the auth_immutable mechanic " +
			"bit, because the flag itself is unset.")
		f.Reasoning = b.String()
	default:
		var b strings.Builder
		b.WriteString("The issuer's flag set is not locked (auth_immutable is unset), " +
			"so it can still change. ")
		if len(powers) == 0 {
			b.WriteString("The issuer holds no freeze or confiscation power today, " +
				"but it may add auth_revocable or auth_clawback_enabled later. ")
		} else {
			fmt.Fprintf(&b,
				"The issuer can %s today, and it may give that power up, or add "+
					"another, later. ", joinPowers(powers))
		}
		b.WriteString("If auth_revocable were added later it would immediately allow " +
			"the issuer to freeze existing trustlines — existing holders are exposed " +
			"as soon as the flag is set. If auth_clawback_enabled were added later, " +
			"under CAP-0035 it would apply only to trustlines opened after the change " +
			"and would not reach trustlines that already exist. Either way this verdict " +
			"is correct for a trustline opened now but is not guaranteed to stay correct " +
			"for one opened later. Severity is unchanged; mutability is reported, not scored.")
		if !signingKnown {
			b.WriteString(" The issuer's signer configuration was not available, so a " +
				"signer lock could neither be ruled in nor out; this finding does not " +
				"claim a signer lock it did not read.")
		}
		f.Reasoning = b.String()
	}

	return f, nil
}

// signerLockState reports whether the issuer account can no longer authorize
// any operation, and whether its signing configuration was read at all.
//
// The account is locked when the master key weight is 0, no other signer
// carries weight, and the medium threshold is non-zero. The medium threshold
// matters: Stellar requires a signature meeting it for SetOptions, which is the
// operation that changes both signers and issuer flags. A medium threshold of 0
// would let anyone submit that operation without a signature, so a zero-weight
// master key alone is not a lock.
//
// known is false when Horizon's signers/thresholds were not read, which is
// distinct from a readable account that is not locked. A caller must never
// infer "no other signers" from an absent signers array.
func signerLockState(s *Subject) (locked, known bool) {
	masterWeight, thresholds, known := s.Issuer.MasterKeyWeight()
	if !known {
		return false, false
	}
	otherWeight, _ := s.Issuer.OtherSignerWeight()
	return masterWeight == 0 && otherWeight == 0 && thresholds.Med > 0, true
}

// holderPowers names the flags that act against a holder who already holds the
// asset, using the severity model's language. auth_required is excluded: it
// gates entry and cannot touch an existing holder (severity Low).
func holderPowers(f horizon.Flags) []string {
	var powers []string
	if f.AuthRevocable {
		powers = append(powers, "freeze an existing holder's balance (auth_revocable)")
	}
	if f.AuthClawbackEnabled {
		powers = append(powers, "confiscate your balance outright and burn it (auth_clawback_enabled)")
	}
	return powers
}
