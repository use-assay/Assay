package main

import (
	"bytes"
	"encoding/base32"
	"os"
	"strings"
	"testing"
)

func fixtureSeed(t *testing.T) string {
	t.Helper()
	payload := append([]byte{seedVersionByte}, bytes.Repeat([]byte{0x42}, 32)...)
	sum := checksum(payload)
	raw := append(payload, sum[:]...)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
}

func fixtureDiff(t *testing.T, name, value string) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(raw), "{{SEED}}", value)
}

func TestValidSecretSeedInDiffIsRejected(t *testing.T) {
	seed := fixtureSeed(t)
	findings, err := ScanDiff(strings.NewReader(fixtureDiff(t, "valid-seed.diff.tmpl", seed)))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(findings))
	}
	if findings[0].Seed != seed {
		t.Fatalf("finding seed = %q, want %q", findings[0].Seed, seed)
	}
	if findings[0].File != ".env" || findings[0].Line != 1 {
		t.Fatalf("finding location = %s:%d, want .env:1", findings[0].File, findings[0].Line)
	}
}

func TestInvalidChecksumIsIgnored(t *testing.T) {
	seed := []byte(fixtureSeed(t))
	if seed[len(seed)-1] == 'A' {
		seed[len(seed)-1] = 'B'
	} else {
		seed[len(seed)-1] = 'A'
	}
	findings, err := ScanDiff(strings.NewReader(fixtureDiff(t, "valid-seed.diff.tmpl", string(seed))))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %d, want 0 for a seed with an invalid checksum", len(findings))
	}
}

func TestPublicAddressInDiffIsIgnored(t *testing.T) {
	raw, err := os.ReadFile("testdata/public-address.diff")
	if err != nil {
		t.Fatal(err)
	}
	findings, err := ScanDiff(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %d, want 0 for a public account address", len(findings))
	}
}

func TestDocumentedEscapeHatchIsHonoured(t *testing.T) {
	seed := fixtureSeed(t)
	diff := "diff --git a/.env b/.env\n--- a/.env\n+++ b/.env\n@@ -0,0 +1 @@\n+SEED=" + seed + " # " + allowMarker + "\n"
	findings, err := ScanDiff(strings.NewReader(diff))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %d, want 0 when the documented marker is present", len(findings))
	}
}

func TestSeedOnLineStartingWithPlusIsRejected(t *testing.T) {
	seed := fixtureSeed(t)
	diff := "diff --git a/.env b/.env\n--- a/.env\n+++ b/.env\n@@ -0,0 +1 @@\n+++" + seed + "\n"
	findings, err := ScanDiff(strings.NewReader(diff))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1 for a seed after leading plus signs", len(findings))
	}
}

func TestSeedEmbeddedInLongerTokenIsIgnored(t *testing.T) {
	seed := fixtureSeed(t)
	findings, err := ScanDiff(strings.NewReader("+++ b/.env\n@@ -0,0 +1 @@\n+X" + seed + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %d, want 0 for a seed embedded in a longer token", len(findings))
	}
}
