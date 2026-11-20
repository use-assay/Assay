package mechanics_test

import (
	"context"
	"testing"

	"github.com/use-assay/assay/internal/mechanics"
)

// TestAuthRequiredIsLow pins the one severity level the captured corpus did not
// exercise: auth_required on its own. It is the worked example cited by
// docs/glossary.md#severity, and it exists so that glossary claim cannot drift
// from the classifier (#411).
//
// The fixture is synthetic (see testdata/synthetic-auth-required-gated/README.md):
// the corpus has real flag-free, revocable and clawback-enabled assets, but no
// real asset whose issuer gates trustlines, and inventing a live-looking capture
// for one would be worse than labelling the construction honestly.
func TestAuthRequiredIsLow(t *testing.T) {
	sub := loadSubject(t, "synthetic-auth-required-gated")

	rep, err := mechanics.NewEngine().Run(context.Background(), sub)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if rep.Base != mechanics.Low {
		t.Errorf("base severity = %v, want low", rep.Base)
	}
	if rep.Severity != mechanics.Low {
		t.Errorf("severity = %v, want low (nothing can escalate a synthetic unlisted issuer)", rep.Severity)
	}
	if rep.Escalated {
		t.Error("escalated = true; low comes from the flag, not reputation")
	}

	var capability *mechanics.Finding
	for i := range rep.Findings {
		if rep.Findings[i].Check == "capability" {
			capability = &rep.Findings[i]
		}
	}
	if capability == nil {
		t.Fatal("no capability finding in the report")
	}
	if capability.Mechanics&mechanics.MechAuthRequired == 0 {
		t.Error("capability finding does not carry the auth_required mechanic")
	}
	if capability.Mechanics&(mechanics.MechAuthRevocable|mechanics.MechClawbackEnabled) != 0 {
		t.Errorf("auth_required-only fixture set a freeze/clawback mechanic: %v",
			capability.Mechanics.Names())
	}

	// The gating flag is a power over entry, not over an existing holder, so
	// the reasoning must not claim the issuer can freeze or confiscate.
	if capability.Reasoning == "" {
		t.Error("capability reasoning is empty; a level is never reported without what it means")
	}
}
