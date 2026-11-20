package temporal

import "github.com/use-assay/assay/internal/mechanics"

// AccountabilityMovement is the accountability axis of a transition: whether
// the party publicly claiming an asset changed between two observations.
//
// Accountability is deliberately independent of severity: a verified domain
// does not make confiscation safer, and losing verification does not give an
// issuer a power it did not hold. That is why this is its own movement rather
// than folded into SeverityMovement. It is still a fact a holder may care about
// — "the issuer used to publish a claim naming this asset and no longer does" —
// and the evidence detectors cannot report it because accountability is not an
// Evidence entry.
//
// From and To are the two sides of the comparison in time order, matching the
// pair carried by Transition. Like the other detectors it reports a State: a
// comparison that could not be made returns Unknown or Missing with no verdict,
// so a consumer cannot read an empty movement as "unchanged" when one side was
// partial.
type AccountabilityMovement struct {
	From mechanics.Accountability `json:"from"`
	To   mechanics.Accountability `json:"to"`
	// Changed is meaningful only when State is Valid.
	Changed bool  `json:"changed"`
	State   State `json:"state"`
	// Reason explains a non-Valid state in plain language.
	Reason string `json:"reason,omitempty"`
}

// AccountabilityTransition derives the accountability movement between two
// observations. It is the same value Between puts in Transition.Accountability,
// exposed as a standalone detector for callers that only need this axis.
func AccountabilityTransition(from, to Observation) AccountabilityMovement {
	return Between(from, to).Accountability
}

// accountabilityMovement projects a transition's ordered pair onto the
// accountability axis. It reads the pair Between already validated rather than
// calling Between again, which would recurse when Between fills this field.
func accountabilityMovement(t Transition) AccountabilityMovement {
	m := AccountabilityMovement{State: t.State, Reason: t.Reason}
	if t.State != Valid {
		return m
	}
	m.From, m.To = t.From.Accountability, t.To.Accountability
	m.Changed = m.From != m.To
	return m
}
