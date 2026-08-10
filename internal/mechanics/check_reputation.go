package mechanics

import (
	"context"
	"fmt"
	"strings"
)

// ReputationCheck folds in StellarExpert's curated reputation data.
//
// Assay does not maintain a scam list, a rating, or a domain blocklist. Those
// exist, they are actively curated, and this check consumes them. Everything it
// produces is attributed Evidence naming StellarExpert and the URL the claim
// came from.
//
// It is the only check permitted to escalate, and it can only ever raise the
// level. A confirmed malicious listing is decisive evidence of abuse. Absence
// from the list is not evidence of anything: most legitimate assets are absent,
// and so is every scam that has not been reported yet.
type ReputationCheck struct{}

// ID implements Check.
func (ReputationCheck) ID() string { return "reputation" }

// Describe implements Check.
func (ReputationCheck) Describe() string {
	return "Consumes StellarExpert's curated address directory and " +
		"malicious-domain blocklist as attributed evidence. Escalates to " +
		"critical on a confirmed listing; never lowers severity."
}

// Run implements Check.
func (c ReputationCheck) Run(_ context.Context, s *Subject) (Finding, error) {
	f := Finding{
		Check:      c.ID(),
		Title:      "Curated reputation signals",
		Severity:   Clear,
		Escalation: true,
		Evidence:   []Evidence{},
	}

	var flagged []string

	if s.Directory != nil {
		tags := strings.Join(s.Directory.Tags, ", ")
		f.Evidence = append(f.Evidence, Evidence{
			Source: "stellar.expert/directory",
			URL:    s.DirectoryURL,
			Claim: fmt.Sprintf("listed as %q (domain %q, tags: %s)",
				s.Directory.Name, s.Directory.Domain, tags),
			RetrievedAt: s.FetchedAt,
		})
		for _, tag := range []string{"malicious", "unsafe"} {
			if s.Directory.HasTag(tag) {
				flagged = append(flagged, fmt.Sprintf("the curated directory tags the issuer %q", tag))
				break
			}
		}
	}

	if s.Blocked != nil {
		f.Evidence = append(f.Evidence, Evidence{
			Source:      "stellar.expert/blocked-domains",
			URL:         s.BlockedURL,
			Claim:       fmt.Sprintf("domain %q blocked=%t", s.Blocked.Domain, s.Blocked.Blocked),
			RetrievedAt: s.FetchedAt,
		})
		if s.Blocked.Blocked {
			flagged = append(flagged, fmt.Sprintf(
				"the malicious-domain blocklist contains %q", s.Blocked.Domain))
		}
	}

	if len(flagged) > 0 {
		f.Severity = Critical
		f.Mechanics = MechBlocklisted
		f.Reasoning = "Escalated to critical because " + joinPowers(flagged) +
			". This is StellarExpert's determination, reported here as their claim " +
			"and not re-derived by Assay. It raises the level regardless of what the " +
			"issuer's flags allow."
		return f, nil
	}

	if len(f.Evidence) == 0 {
		f.Reasoning = "No curated reputation data was available for this issuer. " +
			"That is the normal case and is not a positive signal: absence from a " +
			"scam list is not evidence of safety."
		return f, nil
	}

	f.Reasoning = "Curated sources returned data for this issuer and none of it " +
		"flags the issuer as malicious. Recorded as attributed evidence only: it " +
		"does not lower the capability severity, because a named issuer holds the " +
		"same power over your balance as an anonymous one."
	return f, nil
}
