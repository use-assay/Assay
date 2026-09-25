package mechanics

import (
	"context"
	"strings"
)

// CapabilityCheck derives severity from the issuer's authorization flags.
//
// It is the only check that can set the capability base severity. It reports
// what the issuer is able to do, never who the issuer is.
type CapabilityCheck struct{}

// ID implements Check.
func (CapabilityCheck) ID() string { return "capability" }

// Describe implements Check.
func (CapabilityCheck) Describe() string {
	return "Maps the issuer's authorization flags to a capability severity: " +
		"what the issuer can do to a holder's balance. Says nothing about who " +
		"the issuer is or whether they are likely to use the power."
}

// Run implements Check.
//
// Severity is the highest single capability present, not a sum. auth_revocable
// is a protocol precondition for auth_clawback_enabled (CAP-0035: setting
// clawback without revocable fails with SET_OPTIONS_AUTH_REVOCABLE_REQUIRED),
// so a clawback-capable issuer necessarily also carries revocable. Scoring
// those as two independent signals would double-count a rule the protocol
// enforces, and would make every clawback asset look worse than it is by a
// margin that has nothing to do with its behaviour.
func (c CapabilityCheck) Run(_ context.Context, s *Subject) (Finding, error) {
	f := Finding{
		Check:    c.ID(),
		Title:    "Issuer capability",
		Severity: Clear,
		Evidence: []Evidence{},
	}

	if s.Stat == nil {
		// The flags were never read, so no capability statement exists — not
		// even the statement that there is nothing to state. Reporting Clear
		// here would put the ABI's safest value on a subject nobody assessed
		// (see #23, #25 for that failure shape), so the finding carries the
		// Unevaluated sentinel and marks the report undetermined, which makes
		// it non-attestable at attest.FromReport.
		f.Severity = Unevaluated
		f.Undetermined = true
		f.Reasoning = "Asset not found on the ledger, so no issuer capability could be read: the capability axis is unevaluated, not clear."
		return f, nil
	}

	flags := s.Stat.Flags
	var mech Mechanic
	var powers []string

	if flags.AuthRequired {
		mech |= MechAuthRequired
		powers = append(powers, "decide who is allowed to hold it (auth_required)")
		f.Severity = Low
	}
	if flags.AuthRevocable {
		mech |= MechAuthRevocable
		powers = append(powers, "freeze your balance so you cannot move it (auth_revocable)")
		f.Severity = Medium
	}
	if flags.AuthClawbackEnabled {
		mech |= MechClawbackEnabled
		powers = append(powers, "confiscate your balance outright and burn it, "+
			"without your signature (auth_clawback_enabled)")
		f.Severity = High
	}
	f.Mechanics = mech

	f.Evidence = append(f.Evidence, Evidence{
		Source:      "horizon",
		URL:         horizonAssetURL(s.Asset),
		Claim:       "issuer flags: " + flagSummary(flags),
		RetrievedAt: s.StatFetchedAt,
	})

	var b strings.Builder
	if len(powers) == 0 {
		b.WriteString("The issuer holds no authorization flags. It cannot freeze, " +
			"confiscate, or gate this asset. ")
	} else {
		b.WriteString("The issuer can ")
		b.WriteString(joinPowers(powers))
		b.WriteString(". ")
	}

	// auth_immutable is not a power over holders; it fixes whether the power
	// set can change. It is not scored here and it is not described here:
	// MutabilityCheck reports it as its own finding, so that a reader gets the
	// durability of this verdict without this reasoning having to carry a
	// conditional it cannot score.
	f.Reasoning = b.String()

	return f, nil
}

func joinPowers(p []string) string {
	switch len(p) {
	case 1:
		return p[0]
	case 2:
		return p[0] + ", and " + p[1]
	default:
		return strings.Join(p[:len(p)-1], ", ") + ", and " + p[len(p)-1]
	}
}
