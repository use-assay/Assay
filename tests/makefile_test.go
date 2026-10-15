package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func attestFixture(t *testing.T, withStellar bool) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	makefile, err := os.ReadFile("../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), makefile, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"cut", "sed"} {
		resolved, err := exec.LookPath(tool)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(resolved, filepath.Join(bin, tool)); err != nil {
			t.Fatal(err)
		}
	}
	scanCalls := filepath.Join(dir, "scan-calls")
	stellarCalls := filepath.Join(dir, "stellar-calls")
	scanner := "#!/bin/sh\nprintf 'called\\n' >> \"$ASSAY_TEST_SCAN_CALLS\"\nprintf '1\\t2\\thash\\n'\n"
	if err := os.WriteFile(filepath.Join(dir, "assay-fixture"), []byte(scanner), 0o755); err != nil {
		t.Fatal(err)
	}
	if withStellar {
		stellar := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$ASSAY_TEST_STELLAR_CALLS\"\nprintf 'test-sac\\n'\n"
		if err := os.WriteFile(filepath.Join(bin, "stellar"), []byte(stellar), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir, scanCalls, stellarCalls
}

func runAttest(t *testing.T, dir, asset, scanCalls, stellarCalls string) (string, error) {
	t.Helper()
	makeBin, err := exec.LookPath("make")
	if err != nil {
		t.Fatal(err)
	}
	// Skip only the Go build: exercise the real recipe with isolated command fixtures.
	cmd := exec.Command(makeBin, "--no-print-directory", "-o", "build", "attest", "BINARY=assay-fixture", "ASSET="+asset)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+filepath.Join(dir, "bin"), "ASSAY_TEST_SCAN_CALLS="+scanCalls, "ASSAY_TEST_STELLAR_CALLS="+stellarCalls)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestAttestRequiresStellarBeforeScanning(t *testing.T) {
	dir, scanCalls, stellarCalls := attestFixture(t, false)
	out, err := runAttest(t, dir, "TEST-ISSUER", scanCalls, stellarCalls)
	if err == nil {
		t.Fatal("attest succeeded without stellar")
	}
	if !strings.Contains(out, "stellar CLI is required") || !strings.Contains(out, "https://developers.stellar.org/docs/build/smart-contracts/getting-started/setup") {
		t.Errorf("missing actionable install error: %s", out)
	}
	if _, err := os.Stat(scanCalls); !os.IsNotExist(err) {
		t.Errorf("scanner ran before checking for stellar: %v", err)
	}
}

func TestAttestStillShowsUsageWithoutAnAsset(t *testing.T) {
	dir, scanCalls, stellarCalls := attestFixture(t, false)
	out, err := runAttest(t, dir, "", scanCalls, stellarCalls)
	if err == nil || !strings.Contains(out, "usage: make attest ASSET=CODE-ISSUER") {
		t.Fatalf("expected usage error, got %v: %s", err, out)
	}
}

func TestAttestContinuesWhenStellarIsAvailable(t *testing.T) {
	dir, scanCalls, stellarCalls := attestFixture(t, true)
	out, err := runAttest(t, dir, "TEST-ISSUER", scanCalls, stellarCalls)
	if err != nil {
		t.Fatalf("attest failed with fixture stellar: %v: %s", err, out)
	}
	if calls, err := os.ReadFile(scanCalls); err != nil || string(calls) != "called\n" {
		t.Fatalf("expected one scan, got %q: %v", calls, err)
	}
	calls, err := os.ReadFile(stellarCalls)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(calls), "contract id asset --asset TEST:ISSUER") || !strings.Contains(string(calls), "-- attest --asset test-sac --severity 1 --flags 2 --evidence_hash hash") {
		t.Fatalf("attestation arguments changed: %s", calls)
	}
}
