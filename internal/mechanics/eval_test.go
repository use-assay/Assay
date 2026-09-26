package mechanics_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/use-assay/assay/internal/horizon"
	"github.com/use-assay/assay/internal/mechanics"
	"github.com/use-assay/assay/internal/sep1"
	"github.com/use-assay/assay/internal/stellarexpert"
)

// loadSubject rebuilds a Subject from captured fixtures, using the same
// decoders the live fetchers use. Nothing here touches the network.
func loadSubject(t *testing.T, dir string) *mechanics.Subject {
	t.Helper()
	base := filepath.Join("testdata", dir)

	var stat horizon.AssetStat
	readJSON(t, filepath.Join(base, "asset.json"), &stat)
	var acct horizon.Account
	readJSON(t, filepath.Join(base, "account.json"), &acct)

	// Fixture capture time. A live scan records a separate completion time
	// per source; a fixture replay has no latency, so every source answers at
	// the same instant — which is what the capture itself looked like.
	fetched := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	s := &mechanics.Subject{
		Asset:           mechanics.Asset{Code: stat.AssetCode, Issuer: stat.AssetIssuer},
		Stat:            &stat,
		StatFetchedAt:   fetched,
		Issuer:          &acct,
		IssuerFetchedAt: fetched,
		ScannedAt:       fetched,
	}

	if acct.HomeDomain != "" {
		s.TomlURL = sep1.URLFor(acct.HomeDomain)
		s.TomlAttemptedAt = fetched
	}
	if b, err := os.ReadFile(filepath.Join(base, "stellar.toml")); err == nil {
		doc, err := sep1.Parse(b)
		if err != nil {
			t.Fatalf("parse %s stellar.toml: %v", dir, err)
		}
		doc.URL = s.TomlURL
		s.Toml = doc
	} else if st, err := os.ReadFile(filepath.Join(base, "stellar.toml.status")); err == nil {
		s.TomlErr = "status " + strings.TrimSpace(string(st))
	}

	s.DirectoryURL = "https://api.stellar.expert/explorer/directory/" + stat.AssetIssuer
	s.DirectoryAttemptedAt = fetched
	if _, err := os.Stat(filepath.Join(base, "directory.json")); err == nil {
		var e stellarexpert.DirectoryEntry
		readJSON(t, filepath.Join(base, "directory.json"), &e)
		s.Directory = &e
		s.DirectoryFetchedAt = fetched
	}
	s.BlockedURL = "https://api.stellar.expert/explorer/directory/blocked-domains/"
	if acct.HomeDomain != "" {
		s.BlockedURL += acct.HomeDomain
	}
	s.BlockedAttemptedAt = fetched
	if _, err := os.Stat(filepath.Join(base, "blocked.json")); err == nil {
		var b stellarexpert.BlockedDomain
		readJSON(t, filepath.Join(base, "blocked.json"), &b)
		s.Blocked = &b
		s.BlockedFetchedAt = fetched
	}
	if _, err := os.Stat(filepath.Join(base, "stellar-expert-asset.json")); err == nil {
		var a stellarexpert.Asset
		readJSON(t, filepath.Join(base, "stellar-expert-asset.json"), &a)
		s.ExpertAsset = &a
		s.ExpertAssetURL = "https://api.stellar.expert/explorer/public/asset/" + stat.AssetCode + "-" + stat.AssetIssuer
	}
	return s
}

func readJSON(t *testing.T, path string, out any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

// TestEval is the judgment eval. Detecting a flag is deterministic and
// uninteresting; what these cases measure is whether the severity model
// separates a trap from a legitimate compliance feature, and whether
// escalation fires only on reputation.
//
// See docs/eval.md for the labelling rationale for each subject.
func TestEval(t *testing.T) {
	cases := []struct {
		dir string
		// why states what this subject is supposed to prove.
		why string

		wantBase      mechanics.Severity
		wantSeverity  mechanics.Severity
		wantEscalated bool
		wantAccount   mechanics.Accountability
	}{
		{
			dir:          "aqua-clear-verified",
			why:          "no auth flags at all: the issuer has no power over holders, and a reciprocal domain confirms who it is",
			wantBase:     mechanics.Clear,
			wantSeverity: mechanics.Clear,
			wantAccount:  mechanics.AccountabilityVerified,
		},
		{
			dir:          "shx-clear-flagslocked",
			why:          "no auth flags AND auth_immutable: the issuer can never add freeze or clawback later",
			wantBase:     mechanics.Clear,
			wantSeverity: mechanics.Clear,
			wantAccount:  mechanics.AccountabilityVerified,
		},
		{
			dir: "xrp-clear-unlocked",
			why: "the unlocked counterpart to SHX: no auth flags, but auth_immutable is UNSET, so the " +
				"issuer may add freeze or clawback later. The mutability finding must report that " +
				"without moving base severity off clear.",
			wantBase:     mechanics.Clear,
			wantSeverity: mechanics.Clear,
			wantAccount:  mechanics.AccountabilityVerified,
		},
		{
			dir: "usdc-revocable-regulated",
			why: "a real regulated stablecoin that legitimately uses auth_revocable. It must report " +
				"freeze-capable (medium) on the strength of the flag alone: not discounted to clear " +
				"because Circle issues it, and not escalated because nothing flags it.",
			wantBase:     mechanics.Medium,
			wantSeverity: mechanics.Medium,
			// circle.com does not serve a stellar.toml, so the reciprocal claim
			// genuinely fails. Reported honestly rather than special-cased.
			wantAccount: mechanics.AccountabilityUnverified,
		},
		{
			dir: "berkshire-clawback-scam",
			why: "impersonation asset with clawback: capability alone puts it at high, and the curated " +
				"malicious tag escalates it to critical",
			wantBase:      mechanics.High,
			wantSeverity:  mechanics.Critical,
			wantEscalated: true,
			wantAccount:   mechanics.AccountabilityUnverified,
		},
		{
			dir: "doge-noflags-scam",
			why: "the case that justifies keeping reputation as a separate upward-only axis: a known " +
				"scam asset carrying NO auth flags. Capability is honestly clear, and escalation is " +
				"the only thing that catches it.",
			wantBase:      mechanics.Clear,
			wantSeverity:  mechanics.Critical,
			wantEscalated: true,
			wantAccount:   mechanics.AccountabilityUnverified,
		},
	}

	eng := mechanics.NewEngine()
	for _, tc := range cases {
		t.Run(tc.dir, func(t *testing.T) {
			s := loadSubject(t, tc.dir)
			rep, err := eng.Run(context.Background(), s)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if rep.Base != tc.wantBase {
				t.Errorf("base severity = %v, want %v\nwhy this case exists: %s",
					rep.Base, tc.wantBase, tc.why)
			}
			if rep.Severity != tc.wantSeverity {
				t.Errorf("severity = %v, want %v\nwhy this case exists: %s",
					rep.Severity, tc.wantSeverity, tc.why)
			}
			if rep.Escalated != tc.wantEscalated {
				t.Errorf("escalated = %v, want %v", rep.Escalated, tc.wantEscalated)
			}
			if rep.Accountability != tc.wantAccount {
				t.Errorf("accountability = %v, want %v", rep.Accountability, tc.wantAccount)
			}
		})
	}
}

// TestAccountabilityNeverChangesSeverity is the property the whole model rests
// on. Two subjects with identical issuer flags must classify identically no
// matter how well attributed they are, or a contract gating on severity is
// relying on someone's opinion instead of on ledger mechanics.
func TestAccountabilityNeverChangesSeverity(t *testing.T) {
	eng := mechanics.NewEngine()

	verified := loadSubject(t, "aqua-clear-verified")
	anonymous := loadSubject(t, "aqua-clear-verified")
	// Strip every trace of attribution, leaving the flags untouched.
	anonymous.Issuer.HomeDomain = ""
	anonymous.Toml = nil
	anonymous.Directory = nil
	anonymous.Blocked = nil

	repV, err := eng.Run(context.Background(), verified)
	if err != nil {
		t.Fatal(err)
	}
	repA, err := eng.Run(context.Background(), anonymous)
	if err != nil {
		t.Fatal(err)
	}

	if repV.Severity != repA.Severity {
		t.Errorf("attribution changed severity: verified=%v anonymous=%v",
			repV.Severity, repA.Severity)
	}
	if repV.Accountability == repA.Accountability {
		t.Errorf("accountability should differ between the two subjects, both = %v",
			repV.Accountability)
	}
}

func TestAssetSignalsAreAttributedAndDoNotChangeSeverity(t *testing.T) {
	s := loadSubject(t, "usdc-revocable-regulated")
	s.ExpertAsset = &stellarexpert.Asset{
		Supply:     "3675875656477148",
		Trustlines: stellarexpert.TrustlineCounts{Total: 2431888, Authorized: 2431888, Funded: 703330},
		Rating:     stellarexpert.AssetRating{Age: 10, Activity: 10, Trustlines: 10, Liquidity: 10, Volume7d: 10, Interop: 4, Average: 9},
	}
	s.ExpertAssetURL = "https://api.stellar.expert/explorer/public/asset/USDC-" + s.Asset.Issuer

	rep, err := mechanics.NewEngine().Run(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Base != mechanics.Medium || rep.Severity != mechanics.Medium {
		t.Fatalf("asset metadata changed severity: base=%v severity=%v", rep.Base, rep.Severity)
	}
	var found bool
	for _, evidence := range rep.Evidence {
		if evidence.URL == s.ExpertAssetURL {
			found = true
			if evidence.Source != "StellarExpert" {
				t.Errorf("asset evidence source = %q, want StellarExpert", evidence.Source)
			}
			if !strings.Contains(evidence.Claim, "average=9") || !strings.Contains(evidence.Claim, "supply=3675875656477148") {
				t.Errorf("asset evidence omitted raw metadata: %q", evidence.Claim)
			}
		}
	}
	if !found {
		t.Fatal("asset metadata was not surfaced as evidence")
	}
}

// TestConfiscationImpliesHigh enforces the ABI invariant the contract relies
// on: anything matching ConfiscationMask is at least High.
func TestConfiscationImpliesHigh(t *testing.T) {
	eng := mechanics.NewEngine()
	for _, dir := range []string{
		"aqua-clear-verified", "shx-clear-flagslocked", "xrp-clear-unlocked",
		"usdc-revocable-regulated",
		"berkshire-clawback-scam", "doge-noflags-scam",
	} {
		rep, err := eng.Run(context.Background(), loadSubject(t, dir))
		if err != nil {
			t.Fatal(err)
		}
		if rep.Mechanics&mechanics.ConfiscationMask != 0 && rep.Base < mechanics.High {
			t.Errorf("%s: confiscation-capable but base severity %v < high", dir, rep.Base)
		}
	}
}
