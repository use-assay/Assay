package temporal_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/use-assay/assay/internal/mechanics"
	"github.com/use-assay/assay/internal/temporal"
)

// listingEvidence is the claim internal/mechanics/check_reputation.go writes
// when the curated directory lists an issuer under an adverse tag.
func listingEvidence(claim string) mechanics.Evidence {
	return mechanics.Evidence{
		Source: "stellar.expert/directory",
		URL:    "https://api.stellar.expert/explorer/directory/GA22IDJNHUMC3XKUCCBFNTQIJOUBWINC5GCXHLJ2V6KZ3OWAXCULNQ7P",
		Claim:  claim,
	}
}

// A withdrawn curated listing is the case issue #388 names: final severity
// falls, no capability moved, and the transition is typed as a de-escalation
// naming the source that withdrew rather than leaving the reversal to be found
// by diffing evidence by hand.
func TestDeescalationWithdrawnListing(t *testing.T) {
	before := obs(at(1), mechanics.MechBlocklisted, mechanics.Clear, mechanics.Critical)
	before.Evidence = []mechanics.Evidence{
		listingEvidence(`listed as "DogeCoin" (domain "doge.org", tags: malicious)`),
	}
	after := obs(at(2), 0, mechanics.Clear, mechanics.Clear)

	got := temporal.DeescalationTransition(before, after)
	if got == nil {
		t.Fatal("a severity fall with a withdrawn listing produced no de-escalation")
	}
	if got.From != mechanics.Critical || got.To != mechanics.Clear {
		t.Errorf("deescalation = %v -> %v, want critical -> clear", got.From, got.To)
	}
	if got.Attribution != temporal.AttributionReputation {
		t.Errorf("attribution = %q, want %q: no capability moved", got.Attribution, temporal.AttributionReputation)
	}
	if want := []string{"stellar.expert/directory"}; !slices.Equal(got.Sources, want) {
		t.Errorf("sources = %v, want %v", got.Sources, want)
	}
	if !strings.Contains(got.Reason, "stellar.expert/directory") ||
		!strings.Contains(got.Reason, "withdrew") {
		t.Errorf("reason %q does not name the withdrawing source", got.Reason)
	}

	// The typed form must be on the transition too, not only on the standalone
	// detector: the history API marshals transitions.
	tr := temporal.Between(before, after)
	if tr.Deescalation == nil {
		t.Fatal("the transition carries no de-escalation; the history API would hide it")
	}
	if tr.Deescalation.From != got.From || tr.Deescalation.To != got.To ||
		tr.Deescalation.Attribution != got.Attribution ||
		!slices.Equal(tr.Deescalation.Sources, got.Sources) {
		t.Errorf("transition deescalation = %+v, want %+v", tr.Deescalation, got)
	}
}

// A listing that stays but loses its adverse tag is still the directory
// withdrawing the escalation, so the source is named even though the source
// never stopped answering.
func TestDeescalationListingKeptButAdverseTagRemoved(t *testing.T) {
	before := obs(at(1), mechanics.MechBlocklisted, mechanics.Clear, mechanics.Critical)
	before.Evidence = []mechanics.Evidence{
		listingEvidence(`listed as "DogeCoin" (domain "doge.org", tags: malicious)`),
	}
	after := obs(at(2), 0, mechanics.Clear, mechanics.Clear)
	after.Evidence = []mechanics.Evidence{
		listingEvidence(`listed as "DogeCoin" (domain "doge.org", tags: exchange)`),
	}

	got := temporal.DeescalationTransition(before, after)
	if got == nil {
		t.Fatal("a withdrawn adverse tag produced no de-escalation")
	}
	if want := []string{"stellar.expert/directory"}; !slices.Equal(got.Sources, want) {
		t.Errorf("sources = %v, want %v", got.Sources, want)
	}
}

// An asset list's inclusion claim is never an escalation, so its disappearance
// is not reported as a source that withdrew one: inclusion is not a safety
// signal and absence is not an observation.
func TestDeescalationDoesNotBlameAssetLists(t *testing.T) {
	before := obs(at(1), mechanics.MechBlocklisted, mechanics.Clear, mechanics.Critical)
	before.Evidence = []mechanics.Evidence{{
		Source: "asset-list/wallet",
		URL:    "https://example.com/wallet.toml",
		Claim:  `listed as "DogeCoin" (org "Example") in list "Wallet"`,
	}}
	after := obs(at(2), 0, mechanics.Clear, mechanics.Clear)

	got := temporal.DeescalationTransition(before, after)
	if got == nil {
		t.Fatal("a severity fall produced no de-escalation")
	}
	if len(got.Sources) != 0 {
		t.Errorf("sources = %v, want none: an asset list cannot escalate or withdraw", got.Sources)
	}
}

// A blocklist answer that flips from blocked=true to blocked=false is the other
// withdrawal shape the reputation check writes, and it is named the same way.
func TestDeescalationBlocklistCleared(t *testing.T) {
	before := obs(at(1), mechanics.MechBlocklisted, mechanics.Clear, mechanics.Critical)
	before.Evidence = []mechanics.Evidence{{
		Source: "stellar.expert/blocked-domains",
		URL:    "https://api.stellar.expert/explorer/directory/blocked-domains",
		Claim:  `domain "doge.org" blocked=true`,
	}}
	after := obs(at(2), 0, mechanics.Clear, mechanics.Clear)
	after.Evidence = []mechanics.Evidence{{
		Source: "stellar.expert/blocked-domains",
		URL:    "https://api.stellar.expert/explorer/directory/blocked-domains",
		Claim:  `domain "doge.org" blocked=false`,
	}}

	got := temporal.DeescalationTransition(before, after)
	if got == nil {
		t.Fatal("a cleared blocklist hit produced no de-escalation")
	}
	if want := []string{"stellar.expert/blocked-domains"}; !slices.Equal(got.Sources, want) {
		t.Errorf("sources = %v, want %v", got.Sources, want)
	}
	if got.Attribution != temporal.AttributionReputation {
		t.Errorf("attribution = %q, want %q", got.Attribution, temporal.AttributionReputation)
	}
}

// Removing a capability can lower severity too. It is still a typed
// de-escalation — the direction is a fact a holder needs — but no consumed
// source withdrew a claim, and the type says so instead of inventing a name.
func TestDeescalationCapabilityRemovalNamesNoSource(t *testing.T) {
	before := obs(at(1), mechanics.MechClawbackEnabled, mechanics.High, mechanics.High)
	after := obs(at(2), 0, mechanics.Clear, mechanics.Clear)

	got := temporal.DeescalationTransition(before, after)
	if got == nil {
		t.Fatal("a capability removal that lowered severity produced no de-escalation")
	}
	if got.Attribution != temporal.AttributionCapability {
		t.Errorf("attribution = %q, want %q", got.Attribution, temporal.AttributionCapability)
	}
	if len(got.Sources) != 0 {
		t.Errorf("sources = %v, want none: no source withdrew a claim", got.Sources)
	}
	if got.Reason == "" {
		t.Error("a de-escalation with no named source must still explain itself")
	}
}

// A reputation-only fall whose withdrawal is not visible in the evidence is
// still typed, but it names no source and says the fall is unexplained rather
// than presenting it as a confirmed retraction.
func TestDeescalationUnexplainedReputationFallNamesNoSource(t *testing.T) {
	before := obs(at(1), mechanics.MechBlocklisted, mechanics.Clear, mechanics.Critical)
	after := obs(at(2), 0, mechanics.Clear, mechanics.Clear)

	got := temporal.DeescalationTransition(before, after)
	if got == nil {
		t.Fatal("a reputation-only severity fall produced no de-escalation")
	}
	if len(got.Sources) != 0 {
		t.Errorf("sources = %v, want none: the observations record no withdrawal", got.Sources)
	}
	if !strings.Contains(got.Reason, "unexplained") {
		t.Errorf("reason %q does not say the fall is unexplained", got.Reason)
	}
}

// A held or rising severity is not a de-escalation, and neither is a comparison
// that could not be made: "did not fall" and "could not tell" must not render
// the same.
func TestNoDeescalationWhenSeverityHeldRoseOrUnknown(t *testing.T) {
	held := temporal.DeescalationTransition(
		obs(at(1), 0, mechanics.Clear, mechanics.Clear),
		obs(at(2), 0, mechanics.Clear, mechanics.Clear))
	if held != nil {
		t.Errorf("unchanged severity produced a de-escalation: %+v", held)
	}

	rose := temporal.DeescalationTransition(
		obs(at(1), 0, mechanics.Clear, mechanics.Clear),
		obs(at(2), mechanics.MechClawbackEnabled, mechanics.High, mechanics.High))
	if rose != nil {
		t.Errorf("rising severity produced a de-escalation: %+v", rose)
	}

	after := obs(at(2), 0, mechanics.Clear, mechanics.Clear)
	after.Undetermined = true
	after.UndeterminedChecks = []string{"reputation"}
	if got := temporal.DeescalationTransition(obs(at(1), mechanics.MechBlocklisted, mechanics.Clear, mechanics.Critical), after); got != nil {
		t.Errorf("an undetermined comparison produced a de-escalation: %+v", got)
	}
	if got := temporal.DeescalationTransition(temporal.Observation{},
		obs(at(2), 0, mechanics.Clear, mechanics.Clear)); got != nil {
		t.Errorf("a single observation produced a de-escalation: %+v", got)
	}
}

// The withdrawal is a verdict change, so it must never be reported as an
// evidence-only difference beside it: the severity movement is the headline.
func TestWithdrawnListingIsVerdictChangeNotEvidenceOnly(t *testing.T) {
	before := obs(at(1), mechanics.MechBlocklisted, mechanics.Clear, mechanics.Critical)
	before.Evidence = []mechanics.Evidence{
		listingEvidence(`listed as "DogeCoin" (domain "doge.org", tags: malicious)`),
	}
	after := obs(at(2), 0, mechanics.Clear, mechanics.Clear)

	ev := temporal.EvidenceTransition(before, after)
	if !ev.VerdictChanged || len(ev.Events) != 0 {
		t.Errorf("evidence transition = %+v, want a verdict change with no events", ev)
	}
}
