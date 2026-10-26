package main

import (
	"strings"
	"testing"

	"github.com/use-assay/assay/internal/attest"
)

func TestVerifyPreimageSameEvidenceDifferentVersion(t *testing.T) {
	versions := []string{
		attest.PreimageVersion,
		attest.PreimageVersionCheckSet,
		attest.PreimageVersionNetwork,
		attest.PreimageVersionScanner,
	}
	for sourceIndex, sourceVersion := range versions {
		source := testPreimage(sourceVersion)
		for _, targetVersion := range versions[:sourceIndex] {
			shortVersion := strings.TrimPrefix(targetVersion, "assay-evidence-")
			for _, expectedVersion := range []string{"", targetVersion, shortVersion} {
				outcome, err := verifyPreimage(hashPreimage(testPreimage(targetVersion)), source, expectedVersion)
				if err != nil {
					t.Fatalf("verifyPreimage(%q, %q): %v", sourceVersion, expectedVersion, err)
				}
				if outcome != verifyEncodingDiffers {
					t.Errorf("%s input against %s hash (expected %q): outcome = %q, want %q",
						sourceVersion, targetVersion, expectedVersion, outcome, verifyEncodingDiffers)
				}
			}
		}
	}
}

func TestVerifyPreimageMatching(t *testing.T) {
	versions := []string{
		attest.PreimageVersion,
		attest.PreimageVersionCheckSet,
		attest.PreimageVersionNetwork,
		attest.PreimageVersionScanner,
	}
	for _, version := range versions {
		preimage := testPreimage(version)
		outcome, err := verifyPreimage(hashPreimage(preimage), preimage, "")
		if err != nil {
			t.Fatalf("verifyPreimage(%q): %v", version, err)
		}
		if outcome != verifyMatch {
			t.Errorf("version %q: outcome = %q, want %q", version, outcome, verifyMatch)
		}
	}
}

func TestVerifyPreimageDifferentEvidenceIsMismatch(t *testing.T) {
	preimage := testPreimage(attest.PreimageVersion)
	other := strings.Replace(preimage, "claim", "changed", 1)
	outcome, err := verifyPreimage(hashPreimage(other), preimage, "")
	if err != nil {
		t.Fatalf("verifyPreimage: %v", err)
	}
	if outcome != verifyMismatch {
		t.Fatalf("outcome = %q, want %q", outcome, verifyMismatch)
	}
}

func testPreimage(version string) string {
	lines := []string{version}
	if version == attest.PreimageVersionScanner {
		lines = append(lines, "scanner\tdevel")
	}
	lines = append(lines,
		"asset\tAQUA",
		"severity\t0",
		"base_severity\t0",
		"escalated\tfalse",
		"mechanics\t0",
		"accountability\tverified",
	)
	if version != attest.PreimageVersion {
		lines = append(lines, "checks\tdomain,horizon")
	}
	if version == attest.PreimageVersionNetwork || version == attest.PreimageVersionScanner {
		lines = append(lines, "network\tPublic Global Stellar Network ; September 2015")
	}
	lines = append(lines, "evidence\thorizon\turl\tclaim")
	return strings.Join(lines, "\n") + "\n"
}
