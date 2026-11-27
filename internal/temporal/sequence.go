package temporal

import (
	"sort"

	"github.com/use-assay/assay/internal/mechanics"
)

// All derives the transition across every adjacent pair of a series, ordering
// the series by time first.
//
// Of compares only the two most recent observations, which is the right answer
// to "what changed since I last looked" and the wrong answer to "what has this
// issuer ever been able to do". A capability added and then removed between
// three scans — clear, clawback, clear — gives no change at the end, yet the
// window in which the issuer held clawback is exactly the window in which new
// holders became exposed, and under CAP-0035 that exposure outlives the flag.
// All returns one Transition per adjacent pair so no change older than the last
// pair is invisible.
//
// Each transition is built by Between, so it carries the same State semantics:
// a pair with an undetermined side is Unknown, and a pair sharing a timestamp
// is Unknown, rather than either reading as "no change". Fewer than two
// observations yields no transitions at all — an empty result, not a Missing
// transition — because there is no adjacent pair; Summarize says Missing for
// that case.
func All(observations []Observation) []Transition {
	if len(observations) < 2 {
		return []Transition{}
	}
	ordered := ordered(observations)
	out := make([]Transition, 0, len(ordered)-1)
	for i := 1; i < len(ordered); i++ {
		out = append(out, Between(ordered[i-1], ordered[i]))
	}
	return out
}

// Summary is what a whole sequence of observations says about an issuer's
// capability, rather than what its last pair says.
//
// It exists for the case All makes visible: a power held at some point and
// gone by the end. Latest alone would report that power as absent; Held keeps
// it, and Withdrawn names it as held earlier and absent now, so a reader sees
// the exposure window instead of a clean final state.
type Summary struct {
	Asset mechanics.Asset `json:"asset"`
	// State is Valid when every observation is complete, Unknown when at least
	// one is undetermined, and Missing when there are no observations.
	State State `json:"state"`
	// Reason explains a non-Valid state in plain language.
	Reason string `json:"reason,omitempty"`
	// Count is the number of observations summarised.
	Count int `json:"count"`

	// Held is every capability bit present in at least one observation. A bit
	// present in an undetermined observation still counts: presence was read,
	// and only absence is in doubt when a source did not answer.
	Held      mechanics.Mechanic `json:"held"`
	HeldNames []string           `json:"held_names"`
	// Latest is the capability bits of the most recent observation.
	Latest      mechanics.Mechanic `json:"latest"`
	LatestNames []string           `json:"latest_names"`
	// Withdrawn is Held minus Latest: capability bits the issuer held at some
	// point in the sequence and does not hold in the most recent observation.
	// Under CAP-0035 a trustline that inherited such a bit can still carry it,
	// so a withdrawn bit is not a bit that never mattered.
	Withdrawn      mechanics.Mechanic `json:"withdrawn"`
	WithdrawnNames []string           `json:"withdrawn_names"`
}

// Summarize names every capability bit held at any point in a sequence of
// observations.
//
// Only capability bits are summarised (see CapabilityMask): reported-only
// signals and trustline bits are not powers the issuer holds over a balance.
// The observations need not be ordered; Latest is the newest by time.
func Summarize(observations []Observation) Summary {
	s := Summary{
		Count:          len(observations),
		HeldNames:      []string{},
		LatestNames:    []string{},
		WithdrawnNames: []string{},
	}
	if len(observations) == 0 {
		s.State = Missing
		s.Reason = "no observations: nothing was ever held because nothing was " +
			"ever looked at. This is not a clear result."
		return s
	}

	ordered := ordered(observations)
	last := ordered[len(ordered)-1]
	s.Asset = last.Asset
	s.State = Valid

	var partial []Observation
	for _, o := range ordered {
		s.Held |= o.Mechanics & CapabilityMask
		if o.Undetermined {
			partial = append(partial, o)
		}
	}
	s.Latest = last.Mechanics & CapabilityMask
	s.Withdrawn = s.Held &^ s.Latest
	s.HeldNames = names(s.Held)
	s.LatestNames = names(s.Latest)
	s.WithdrawnNames = names(s.Withdrawn)

	if len(partial) > 0 {
		var checks []string
		seen := map[string]bool{}
		for _, o := range partial {
			for _, c := range o.UndeterminedChecks {
				if !seen[c] {
					seen[c] = true
					checks = append(checks, c)
				}
			}
		}
		named := "an unnamed check"
		if len(checks) > 0 {
			named = joinChecks(checks)
		}
		s.State = Unknown
		s.Reason = "one or more observations are undetermined (" + named +
			" did not complete), so a capability absent from them may only be " +
			"unread. The bits named as held were read and are reported; the " +
			"set may be incomplete."
	}
	return s
}

// ordered returns a time-ordered copy of observations; the input is not
// modified. The sort is stable so observations sharing a timestamp keep the
// order the caller gave them.
func ordered(observations []Observation) []Observation {
	out := append([]Observation(nil), observations...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}

// names is Mechanic.Names with a non-nil result, so an empty set encodes as []
// rather than null and a consumer never has to tell the two apart.
func names(m mechanics.Mechanic) []string {
	n := m.Names()
	if n == nil {
		return []string{}
	}
	return n
}
