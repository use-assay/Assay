package mechanics_test

import (
	"context"
	"strings"
	"testing"

	"github.com/use-assay/assay/internal/mechanics"
)

// TestSACAdminMovedIsEvidenceOnly is the #356 acceptance test. When the
// Stellar Asset Contract admin differs from the issuer, the capability finding
// must state the fact, cite it, and leave severity and mechanics exactly as the
// issuer flags alone dictate. When the admin still is the issuer, it must add
// nothing — both because there is nothing new to say and because a claim on
// every report would change every attestation's evidence_hash.
func TestSACAdminMovedIsEvidenceOnly(t *testing.T) {
	cases := []struct {
		name         string
		fixture      string
		wantMoved    bool
		wantAdmin    string
		wantSeverity mechanics.Severity
	}{
		{
			name:         "admin moved to a contract",
			fixture:      "synthetic-sac-admin-moved",
			wantMoved:    true,
			wantAdmin:    "CDFQ5K2BPKB7BWNW2SJPGIIK5OOFQIR434MOX5YYBDKAN3M5CFVJKHR2",
			wantSeverity: mechanics.Clear,
		},
		{
			name:         "admin is still the issuer",
			fixture:      "synthetic-sac-admin-issuer",
			wantMoved:    false,
			wantSeverity: mechanics.Clear,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := loadSubject(t, tc.fixture)

			baseline := *s
			baseline.SACAdmin = ""
			baseline.SACAddress = ""
			baseline.SACAdminURL = ""

			without, err := mechanics.CapabilityCheck{}.Run(context.Background(), &baseline)
			if err != nil {
				t.Fatalf("capability without SAC admin: %v", err)
			}

			f, err := mechanics.CapabilityCheck{}.Run(context.Background(), s)
			if err != nil {
				t.Fatalf("capability: %v", err)
			}

			if f.Severity != tc.wantSeverity || f.Severity != without.Severity {
				t.Errorf("severity = %v, want %v (unchanged from %v)",
					f.Severity, tc.wantSeverity, without.Severity)
			}
			if f.Mechanics != without.Mechanics {
				t.Errorf("mechanics = %v, want %v (SAC admin is not a capability bit)",
					f.Mechanics.Names(), without.Mechanics.Names())
			}

			moved := strings.Contains(f.Reasoning, "administered by")
			if moved != tc.wantMoved {
				t.Fatalf("reasoning reports a moved admin = %v, want %v:\n%s", moved, tc.wantMoved, f.Reasoning)
			}
			if !tc.wantMoved {
				if len(f.Evidence) != len(without.Evidence) {
					t.Errorf("issuer-admin case added evidence: %d vs baseline %d",
						len(f.Evidence), len(without.Evidence))
				}
				return
			}

			if !strings.Contains(f.Reasoning, "CAP-46-06") {
				t.Errorf("reasoning must cite CAP-46-06: %s", f.Reasoning)
			}
			if !strings.Contains(f.Reasoning, "does not change severity") {
				t.Errorf("reasoning must state that severity is unchanged: %s", f.Reasoning)
			}
			if !strings.Contains(f.Reasoning, tc.wantAdmin) {
				t.Errorf("reasoning must name the admin %s: %s", tc.wantAdmin, f.Reasoning)
			}

			var found bool
			for _, e := range f.Evidence {
				if e.Source == "soroban-rpc/sac-admin" {
					found = true
					if !strings.Contains(e.Claim, tc.wantAdmin) {
						t.Errorf("evidence claim %q does not name the admin", e.Claim)
					}
					if e.URL == "" {
						t.Error("evidence has no URL to re-check")
					}
				}
			}
			if !found {
				t.Errorf("no soroban-rpc/sac-admin evidence in the moved-admin finding: %v", f.Evidence)
			}
		})
	}
}

// TestSACAdminUnreadableAddsNothing pins the negative path: a subject whose
// admin could not be read (no sac-admin.json) must not be described as though
// the issuer were the admin, and must not gain evidence.
func TestSACAdminUnreadableAddsNothing(t *testing.T) {
	s := loadSubject(t, "usdc-revocable-regulated")
	if s.SACAdmin != "" || s.SACAddress != "" {
		t.Fatalf("fixture unexpectedly carries a SAC admin: %q %q", s.SACAdmin, s.SACAddress)
	}
	f, err := mechanics.CapabilityCheck{}.Run(context.Background(), s)
	if err != nil {
		t.Fatalf("capability: %v", err)
	}
	if strings.Contains(f.Reasoning, "administered by") {
		t.Errorf("a finding claimed an admin it did not read: %s", f.Reasoning)
	}
	for _, e := range f.Evidence {
		if e.Source == "soroban-rpc/sac-admin" {
			t.Errorf("unexpected SAC admin evidence without a read: %v", e)
		}
	}
}
