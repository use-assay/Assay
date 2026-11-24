package temporal

import (
	"slices"
	"sort"
	"strings"

	"github.com/use-assay/assay/internal/mechanics"
)

// Deescalation is a severity decrease with the source that withdrew it named.
//
// A final severity can fall because a consumed source withdrew the claim that
// raised it — StellarExpert dropping a listing, or the blocklist no longer
// containing the issuer's domain. Across reports that is legitimate: the new
// attestation asserts what the scanner can see now, and refusing to ever lower
// an escalation would leave the registry unable to correct a wrong one
// (docs/threat-model.md, attack class 4). What is *not* acceptable is that the
// reversal be invisible. A consumer reading only the current attestation sees a
// lower level with no indication that anything was ever higher, and the
// historical pair alone requires diffing evidence by hand to see why.
//
// This type makes the reversal itself observable and attributed: it is emitted
// exactly when final severity falls between two complete observations, and it
// names the evidence source(s) whose claims withdrew the escalation. The
// detection is deliberately narrow — it reads the same claims
// internal/mechanics/check_reputation.go writes, and it names a source only
// when that source's own evidence moved from an affirmative adverse claim to
// no claim. It never guesses at a source it cannot see move.
type Deescalation struct {
	// From/To are the final severity before and after. To is strictly lower.
	From mechanics.Severity `json:"from"`
	To   mechanics.Severity `json:"to"`

	// Attribution names the axis that produced the fall, using the same
	// vocabulary as Transition.Attribution: "reputation" for a withdrawn
	// listing, "capability" when a flag was removed, "both" when both moved.
	Attribution Attribution `json:"attribution"`

	// Sources names the evidence sources that withdrew an escalation-causing
	// claim, sorted and deduplicated. It is empty when severity fell but no
	// consumed source's evidence records a withdrawal — Reason then says so
	// rather than letting an empty list read as "no source was involved".
	Sources []string `json:"sources"`

	// Reason is a plain-language sentence: what fell, from where to where, and
	// which sources (if any) were observed to withdraw.
	Reason string `json:"reason,omitempty"`
}

// DeescalationTransition reports the de-escalation between two observations, or
// nil when final severity did not fall. It is meaningful only on a Valid
// comparison: Unknown and Missing are no answer, not a fall to zero.
func DeescalationTransition(from, to Observation) *Deescalation {
	return deescalation(Between(from, to))
}

// deescalation projects the fall out of a transition whose state and
// attribution are already settled. The caller holds the ordered pair, so the
// evidence comparison cannot be inverted by argument order.
func deescalation(t Transition) *Deescalation {
	if t.State != Valid || t.SeverityAfter >= t.SeverityBefore {
		return nil
	}
	sources := withdrawnEscalationSources(t.From.Evidence, t.To.Evidence)
	d := &Deescalation{
		From:        t.SeverityBefore,
		To:          t.SeverityAfter,
		Attribution: t.Attribution,
		Sources:     sources,
	}
	switch {
	case len(sources) > 0:
		d.Reason = "Final severity fell from " + t.SeverityBefore.String() +
			" to " + t.SeverityAfter.String() + " because " +
			joinSources(sources) + " withdrew the claim that had escalated it. " +
			"The fall is reported as a de-escalation naming that source, not as " +
			"an absence of change: a lower level now says the source no longer " +
			"asserts abuse, and says nothing about the interval when it did."
	case t.Attribution == AttributionReputation:
		d.Reason = "Final severity fell from " + t.SeverityBefore.String() +
			" to " + t.SeverityAfter.String() + " with reputation as the only " +
			"axis that moved, but no consumed source's evidence records the " +
			"withdrawal, so no source is named. Treat the fall as unexplained " +
			"rather than as a confirmed retraction."
	default:
		d.Reason = "Final severity fell from " + t.SeverityBefore.String() +
			" to " + t.SeverityAfter.String() + ", attributed to " +
			string(t.Attribution) + "; no consumed source withdrew a claim, so " +
			"no source is named."
	}
	return d
}

// withdrawnEscalationSources names the sources whose evidence moved from an
// affirmative escalation claim to none, using the ordered pair's evidence.
//
// Only two shapes exist in the codebase, both written by
// internal/mechanics/check_reputation.go:
//
//   - a curated directory listing: "listed as ..." (the adverse tag that
//     escalates is inside the claim; Assay does not re-derive it here)
//   - a blocklist hit: "domain ... blocked=true"
//
// A directory that used to list the asset and now answers without listing it
// appears as EvidenceRemoved, because the source's evidence entry is only
// written when the source has an answer. A blocklist whose answer flipped from
// blocked=true to blocked=false appears as EvidenceChanged. Both are
// withdrawals; a source that merely stopped answering is not, because a failed
// fetch makes the observation undetermined and cannot yield a Valid comparison
// in the first place.
//
// This is a prefix/suffix match on the checks' own claim format, the same
// deliberately narrow convention isFailure uses. It classifies no arbitrary
// natural-language claim.
func withdrawnEscalationSources(from, to []mechanics.Evidence) []string {
	var sources []string
	seen := map[string]bool{}
	for _, e := range evidenceEvents(from, to) {
		if !withdrewEscalation(e) || seen[e.Source] {
			continue
		}
		seen[e.Source] = true
		sources = append(sources, e.Source)
	}
	sort.Strings(sources)
	return sources
}

// withdrewEscalation reports whether one evidence event moved an affirmative
// adverse claim to no claim.
func withdrewEscalation(e EvidenceEvent) bool {
	switch e.Kind {
	case EvidenceRemoved:
		return escalatingClaim(e.Before.Claim)
	case EvidenceChanged:
		return escalatingClaim(e.Before.Claim) && !escalatingClaim(e.After.Claim)
	default:
		return false
	}
}

// escalatingClaim reports whether a reputation check's claim asserts an adverse
// determination. It reads the two shapes internal/mechanics/check_reputation.go
// writes, so a withdrawal is judged by the same vocabulary that escalates:
//
//   - `domain %q blocked=true` — the blocklist hit.
//   - `listed as %q (domain %q, tags: %s)` — the directory listing, which
//     escalates only when its tag list contains an adverse tag. The tag
//     vocabulary is mechanics.AdverseDirectoryTags, imported rather than
//     restated, so the two cannot drift apart.
//
// An asset list's `listed as ... in list ...` claim carries no `, tags:` marker
// and so never escalates, which is correct: inclusion is not a safety signal.
// A listing that stays but loses its adverse tag therefore changes from an
// escalating claim to a non-escalating one, and names the directory as the
// source that withdrew the escalation.
func escalatingClaim(claim string) bool {
	if strings.HasSuffix(claim, "blocked=true") {
		return true
	}
	if !strings.HasPrefix(claim, "listed as ") {
		return false
	}
	const tagsMarker = ", tags: "
	i := strings.Index(claim, tagsMarker)
	if i < 0 {
		return false
	}
	tags := strings.TrimSuffix(claim[i+len(tagsMarker):], ")")
	for _, tag := range strings.Split(tags, ", ") {
		if slices.Contains(mechanics.AdverseDirectoryTags, strings.TrimSpace(tag)) {
			return true
		}
	}
	return false
}

// joinSources renders a source list as prose, keeping the last item joined with
// "and" so a two-source withdrawal does not read like a list fragment.
func joinSources(sources []string) string {
	switch len(sources) {
	case 0:
		return "no source"
	case 1:
		return sources[0]
	default:
		return strings.Join(sources[:len(sources)-1], ", ") + " and " + sources[len(sources)-1]
	}
}
