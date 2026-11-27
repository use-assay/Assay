package mechanics_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/use-assay/assay/internal/horizon"
	"github.com/use-assay/assay/internal/mechanics"
)

// fixtureDirs is every labelled subject in the eval set. Kept here so the
// mutability tests fail loudly if a fixture is dropped from the set.
var fixtureDirs = []string{
	"aqua-clear-verified",
	"shx-clear-flagslocked",
	"xrp-clear-unlocked",
	"usdc-revocable-regulated",
	"berkshire-clawback-scam",
	"doge-noflags-scam",
	"zzz-signer-locked",
	"usdc-signers-hold-power",
}

// mutSubject builds a Subject from flags directly. The locked-with-danger state
// has no live fixture that carries auth_immutable and clawback at once, so it
// is constructed rather than captured — the same approach check_trustline_test
// uses for the states with no pubnet specimen.
func mutSubject(flags horizon.Flags) *mechanics.Subject {
	const issuer = "GBXRPL45NPHCVMFFAYZVUVFFVKSIZ362ZXFP7I2ETNQ3QKZMFLPRDTD5"
	return &mechanics.Subject{
		Asset:     mechanics.Asset{Code: "TESTTKN", Issuer: issuer},
		Stat:      &horizon.AssetStat{AssetCode: "TESTTKN", AssetIssuer: issuer, Flags: flags},
		Issuer:    &horizon.Account{AccountID: issuer, Flags: flags},
		ScannedAt: time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC),
	}
}

// signedSubject builds a Subject whose issuer publishes signing state, so the
// signer-locked state can be exercised without inventing a live account.
func signedSubject(flags horizon.Flags, masterWeight int, otherSigners []horizon.Signer, medThreshold int) *mechanics.Subject {
	const issuer = "GBXRPL45NPHCVMFFAYZVUVFFVKSIZ362ZXFP7I2ETNQ3QKZMFLPRDTD5"
	signers := []horizon.Signer{{Key: issuer, Weight: masterWeight, Type: "ed25519_public_key"}}
	signers = append(signers, otherSigners...)
	return &mechanics.Subject{
		Asset: mechanics.Asset{Code: "TESTTKN", Issuer: issuer},
		Stat:  &horizon.AssetStat{AssetCode: "TESTTKN", AssetIssuer: issuer, Flags: flags},
		Issuer: &horizon.Account{
			AccountID:    issuer,
			Flags:        flags,
			Signers:      signers,
			Thresholds:   horizon.AccountThresholds{Low: medThreshold, Med: medThreshold, High: medThreshold},
			SigningKnown: true,
		},
		ScannedAt: time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC),
	}
}

func runMut(t *testing.T, s *mechanics.Subject) mechanics.Finding {
	t.Helper()
	f, err := mechanics.MutabilityCheck{}.Run(context.Background(), s)
	if err != nil {
		t.Fatalf("MutabilityCheck.Run: %v", err)
	}
	return f
}

// TestMutabilityReportsThreeStates is the core of the check: the three states
// the assignment distinguishes must each get their own reasoning, and none of
// them may be a severity.
func TestMutabilityReportsThreeStates(t *testing.T) {
	cases := []struct {
		name       string
		flags      horizon.Flags
		wantLocked bool
		// wantWords are phrases the reasoning must contain for this state, so a
		// state collapsing into another is caught rather than merely rendering
		// differently.
		wantWords []string
	}{
		{
			name:       "locked with no dangerous flags",
			flags:      horizon.Flags{AuthImmutable: true},
			wantLocked: true,
			wantWords:  []string{"locked", "durable", "can never be added"},
		},
		{
			name: "locked with clawback",
			flags: horizon.Flags{
				AuthImmutable: true, AuthRevocable: true, AuthClawbackEnabled: true,
			},
			wantLocked: true,
			wantWords:  []string{"locked", "permanent", "cannot be cleared", "confiscate"},
		},
		{
			name:       "not locked",
			flags:      horizon.Flags{},
			wantLocked: false,
			wantWords:  []string{"not locked", "auth_revocable", "existing", "auth_clawback_enabled", "CAP-0035"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := runMut(t, mutSubject(tc.flags))

			if f.Severity != mechanics.Clear {
				t.Errorf("mutability must never set severity; got %v", f.Severity)
			}
			if f.Escalation {
				t.Error("mutability must never set Escalation")
			}
			if f.Undetermined {
				t.Error("mutability over readable flags must not be undetermined")
			}

			locked := f.Mechanics&mechanics.MechFlagsLocked != 0
			if locked != tc.wantLocked {
				t.Errorf("MechFlagsLocked = %v, want %v", locked, tc.wantLocked)
			}

			for _, want := range tc.wantWords {
				if !strings.Contains(f.Reasoning, want) {
					t.Errorf("reasoning for %q is missing %q:\n%s", tc.name, want, f.Reasoning)
				}
			}
		})
	}
}

// TestMutabilitySignerLockedState is the fourth state (#359): an issuer whose
// master key weight is 0, with no other signer carrying weight and a non-zero
// medium threshold, can never authorize another operation. Its flags can never
// change even though auth_immutable is unset, so the finding must say so —
// without claiming the auth_immutable bit and without moving severity.
func TestMutabilitySignerLockedState(t *testing.T) {
	f := runMut(t, signedSubject(horizon.Flags{}, 0, nil, 1))

	if !strings.Contains(f.Reasoning, "locked by its signer configuration") {
		t.Errorf("signer-locked reasoning is missing the state: %s", f.Reasoning)
	}
	if !strings.Contains(f.Reasoning, "can never change") {
		t.Errorf("signer-locked reasoning must state the durability: %s", f.Reasoning)
	}
	if f.Severity != mechanics.Clear {
		t.Errorf("signer-locked must never set severity; got %v", f.Severity)
	}
	if f.Escalation {
		t.Error("signer-locked must never escalate")
	}
	if f.Mechanics&mechanics.MechFlagsLocked != 0 {
		t.Error("signer-locked must not claim the auth_immutable mechanic bit")
	}
	if f.Undetermined {
		t.Error("a readable signer-locked account is determined")
	}
}

// TestMutabilitySignerLockedKeepsExistingPower covers the other direction: a
// signer-locked issuer that already holds clawback keeps it, and the reasoning
// says the power is permanent rather than pretending the lock makes it safer.
func TestMutabilitySignerLockedKeepsExistingPower(t *testing.T) {
	f := runMut(t, signedSubject(horizon.Flags{AuthRevocable: true, AuthClawbackEnabled: true}, 0, nil, 1))

	if !strings.Contains(f.Reasoning, "locked by its signer configuration") {
		t.Errorf("reasoning is missing the signer-locked state: %s", f.Reasoning)
	}
	if !strings.Contains(f.Reasoning, "confiscate") || !strings.Contains(f.Reasoning, "permanent") {
		t.Errorf("reasoning must keep the existing power and call it permanent: %s", f.Reasoning)
	}
	if f.Severity != mechanics.Clear {
		t.Errorf("mutability must not restate capability severity; got %v", f.Severity)
	}
}

// TestMutabilitySignerLockedCounterparts pins the cases that must NOT count as
// locked by signers: another signer with weight can still authorize, a zero
// medium threshold lets the operation run unsigned, and unread signing state is
// unknown rather than locked.
func TestMutabilitySignerLockedCounterparts(t *testing.T) {
	cases := []struct {
		name        string
		subject     *mechanics.Subject
		wantLocked  bool
		wantPhrases []string
	}{
		{
			name: "another signer with weight can authorize",
			subject: signedSubject(horizon.Flags{}, 0, []horizon.Signer{
				{Key: "GAUKFOOO2BQZ5Z5QZ5Z5QZ5Z5QZ5Z5QZ5Z5QZ5Z5QZ5Z5QZ5Z5QZ5Z5", Weight: 1, Type: "ed25519_public_key"},
			}, 1),
			wantLocked:  false,
			wantPhrases: []string{"not locked"},
		},
		{
			name:        "zero medium threshold needs no signature",
			subject:     signedSubject(horizon.Flags{}, 0, nil, 0),
			wantLocked:  false,
			wantPhrases: []string{"not locked"},
		},
		{
			name: "signing state was not read",
			subject: &mechanics.Subject{
				Asset:     mechanics.Asset{Code: "TESTTKN", Issuer: "GBXRPL45NPHCVMFFAYZVUVFFVKSIZ362ZXFP7I2ETNQ3QKZMFLPRDTD5"},
				Stat:      &horizon.AssetStat{Flags: horizon.Flags{}},
				Issuer:    &horizon.Account{AccountID: "GBXRPL45NPHCVMFFAYZVUVFFVKSIZ362ZXFP7I2ETNQ3QKZMFLPRDTD5"},
				ScannedAt: time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC),
			},
			wantLocked:  false,
			wantPhrases: []string{"not locked", "signer configuration was not available"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := runMut(t, tc.subject)
			if f.Severity != mechanics.Clear {
				t.Errorf("mutability must never set severity; got %v", f.Severity)
			}
			if f.Mechanics&mechanics.MechFlagsLocked != 0 {
				t.Error("a signer lock must never set the auth_immutable bit")
			}
			if strings.Contains(f.Reasoning, "locked by its signer configuration") != tc.wantLocked {
				t.Errorf("signer-locked reasoning = %v, want %v:\n%s",
					strings.Contains(f.Reasoning, "locked by its signer configuration"),
					tc.wantLocked, f.Reasoning)
			}
			for _, want := range tc.wantPhrases {
				if !strings.Contains(f.Reasoning, want) {
					t.Errorf("reasoning is missing %q:\n%s", want, f.Reasoning)
				}
			}
		})
	}
}

// TestMutabilitySignerStateNeverChangesSeverity is the property the acceptance
// criteria require: across every combination of flags and signing state, the
// new fourth state can only change reasoning. Neither the mutability finding
// nor the engine's base/final severity may move.
func TestMutabilitySignerStateNeverChangesSeverity(t *testing.T) {
	withoutMutability := &mechanics.Engine{
		Checks: []mechanics.Check{
			mechanics.CapabilityCheck{},
			mechanics.DomainCheck{},
			mechanics.ReputationCheck{},
		},
		AllowPartialCheckSet: true,
	}
	withMutability := mechanics.NewEngine()

	signingStates := []struct {
		name         string
		masterWeight int
		others       []horizon.Signer
		med          int
	}{
		{name: "signer-locked", masterWeight: 0, med: 1},
		{name: "master key active", masterWeight: 1, med: 1},
		{name: "other signer", masterWeight: 0, med: 1, others: []horizon.Signer{
			{Key: "GAUKFOOO2BQZ5Z5QZ5Z5QZ5Z5QZ5Z5QZ5Z5QZ5Z5QZ5Z5QZ5Z5QZ5Z5", Weight: 1, Type: "ed25519_public_key"},
		}},
		{name: "zero threshold", masterWeight: 0, med: 0},
	}

	for mask := 0; mask < 16; mask++ {
		flags := horizon.Flags{
			AuthRequired:        mask&1 != 0,
			AuthRevocable:       mask&2 != 0,
			AuthImmutable:       mask&4 != 0,
			AuthClawbackEnabled: mask&8 != 0,
		}
		for _, st := range signingStates {
			t.Run(fmt.Sprintf("mask%d/%s", mask, st.name), func(t *testing.T) {
				s := signedSubject(flags, st.masterWeight, st.others, st.med)
				f := runMut(t, s)
				if f.Severity != mechanics.Clear || f.Escalation || f.Undetermined {
					t.Fatalf("mutability finding carried severity=%v escalation=%v undetermined=%v",
						f.Severity, f.Escalation, f.Undetermined)
				}

				rep, err := withMutability.Run(context.Background(), s)
				if err != nil {
					t.Fatalf("engine with mutability: %v", err)
				}
				before, err := withoutMutability.Run(context.Background(), s)
				if err != nil {
					t.Fatalf("engine without mutability: %v", err)
				}
				if rep.Base != before.Base || rep.Severity != before.Severity || rep.Escalated != before.Escalated {
					t.Errorf("severity moved: with=(%v,%v,%v) without=(%v,%v,%v)",
						rep.Base, rep.Severity, rep.Escalated, before.Base, before.Severity, before.Escalated)
				}
			})
		}
	}
}

// TestMutabilityUnreadableFlagsIsUndetermined enforces the repo-wide rule that
// "we could not check" and "this is fine" must never render the same: with no
// asset record there are no flags, so the answer is unknown, not unlocked.
func TestMutabilityUnreadableFlagsIsUndetermined(t *testing.T) {
	f := runMut(t, &mechanics.Subject{
		Asset:     mechanics.Asset{Code: "TESTTKN", Issuer: "GBXRPL45NPHCVMFFAYZVUVFFVKSIZ362ZXFP7I2ETNQ3QKZMFLPRDTD5"},
		ScannedAt: time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC),
	})
	if !f.Undetermined {
		t.Fatal("a missing asset record must mark the mutability finding undetermined")
	}
	if f.Severity != mechanics.Clear {
		t.Errorf("undetermined must not invent a severity; got %v", f.Severity)
	}
}

// TestMutabilityLeavesBaseSeverityUnchanged is the acceptance property. Adding
// this check must not move the capability base severity for any subject, locked
// or unlocked. Two independent checks:
//
//   - the base severity equals the value documented before this check existed,
//     so a drift is caught even if the engine's own comparison were wrong; and
//   - the full engine produces the same base and final severity as the same
//     engine with the mutability check removed.
func TestMutabilityLeavesBaseSeverityUnchanged(t *testing.T) {
	// Baselines are the capability-only base severities for the eval subjects
	// before this check was added (docs/eval.md).
	baseline := map[string]mechanics.Severity{
		"aqua-clear-verified":      mechanics.Clear,
		"shx-clear-flagslocked":    mechanics.Clear,
		"xrp-clear-unlocked":       mechanics.Clear,
		"usdc-revocable-regulated": mechanics.Medium,
		"berkshire-clawback-scam":  mechanics.High,
		"doge-noflags-scam":        mechanics.Clear,
		"zzz-signer-locked":        mechanics.Clear,
		"usdc-signers-hold-power":  mechanics.Medium,
	}

	withoutMutability := &mechanics.Engine{
		Checks: []mechanics.Check{
			mechanics.CapabilityCheck{},
			mechanics.DomainCheck{},
			mechanics.ReputationCheck{},
		},
		AllowPartialCheckSet: true,
	}
	withMutability := mechanics.NewEngine()

	for _, dir := range fixtureDirs {
		t.Run(dir, func(t *testing.T) {
			s := loadSubject(t, dir)

			rep, err := withMutability.Run(context.Background(), s)
			if err != nil {
				t.Fatalf("engine run: %v", err)
			}
			if rep.Base != baseline[dir] {
				t.Errorf("base severity = %v, want %v (unchanged by the mutability check)",
					rep.Base, baseline[dir])
			}

			before, err := withoutMutability.Run(context.Background(), s)
			if err != nil {
				t.Fatalf("engine run (without mutability): %v", err)
			}
			if rep.Base != before.Base {
				t.Errorf("base severity moved: with mutability=%v without=%v", rep.Base, before.Base)
			}
			if rep.Severity != before.Severity {
				t.Errorf("final severity moved: with mutability=%v without=%v", rep.Severity, before.Severity)
			}
			if rep.Escalated != before.Escalated {
				t.Errorf("escalation moved: with mutability=%v without=%v", rep.Escalated, before.Escalated)
			}
		})
	}
}

// TestLockedMechanicBitSurvivesMove: auth_immutable moved out of the capability
// check and into this one. The report's bitset is the contract-relevant output,
// so it must not change: a locked asset still reports the bit.
func TestLockedMechanicBitSurvivesMove(t *testing.T) {
	rep := run(t, loadSubject(t, "shx-clear-flagslocked"))
	if rep.Mechanics&mechanics.MechFlagsLocked == 0 {
		t.Error("auth_immutable must still appear in the report mechanics after the move")
	}

	rep = run(t, loadSubject(t, "xrp-clear-unlocked"))
	if rep.Mechanics&mechanics.MechFlagsLocked != 0 {
		t.Error("an unlocked asset must not carry the auth_immutable bit")
	}
}

// TestMutabilityIsRegistered: the check must actually be part of the default
// engine, or every property above is about a check nobody runs.
func TestMutabilityIsRegistered(t *testing.T) {
	for _, c := range mechanics.NewEngine().Checks {
		if c.ID() == "mutability" {
			return
		}
	}
	t.Fatal("mutability check is not registered in NewEngine()")
}
