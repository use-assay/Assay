package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/use-assay/assay/internal/attest"
	"github.com/use-assay/assay/internal/mechanics"
)

type mockScanner struct {
	report *mechanics.Report
	err    error
}

func (m mockScanner) Scan(ctx context.Context, asset mechanics.Asset) (*mechanics.Report, error) {
	return m.report, m.err
}

type mockRegistry struct {
	safety *OnChainSafety
	err    error
}

func (m mockRegistry) Read(ctx context.Context, asset mechanics.Asset) (*OnChainSafety, error) {
	return m.safety, m.err
}

func TestVerifyCommand(t *testing.T) {
	asset := mechanics.Asset{Code: "AQUA", Issuer: "GBNZILSTVQZ4R7IKQDGHYGY2QXL5QOFJYQMXPKWRRM5PAV7Y4M67AQUA"}
	now := time.Now()
	
	validRep := &mechanics.Report{
		Asset:    asset,
		Severity: mechanics.Clear,
		Base:     mechanics.Clear,
	}
	params, _ := attest.FromReport(validRep)
	
	validOnChain := &OnChainSafety{
		Severity:     params.Severity,
		Flags:        params.Flags,
		EvidenceHash: params.EvidenceHash,
		AttestedAt:   uint64(now.Unix()),
	}

	undeterminedRep := &mechanics.Report{
		Asset:        asset,
		Undetermined: true,
	}

	cases := []struct {
		name    string
		scanner Scanner
		reader  RegistryReader
		args    []string
		wantErr string
		outCode int
	}{
		{
			name:    "match",
			scanner: mockScanner{report: validRep},
			reader:  mockRegistry{safety: validOnChain},
			args:    []string{asset.String()},
			wantErr: "",
			outCode: 0,
		},
		{
			name:    "undetermined",
			scanner: mockScanner{report: undeterminedRep},
			reader:  mockRegistry{safety: nil},
			args:    []string{asset.String()},
			wantErr: "scan undetermined",
			outCode: 2,
		},
		{
			name:    "absent",
			scanner: mockScanner{report: validRep},
			reader:  mockRegistry{safety: nil},
			args:    []string{asset.String()},
			wantErr: "no attestation on-chain",
			outCode: 3,
		},
		{
			name:    "mismatch",
			scanner: mockScanner{report: validRep},
			reader:  mockRegistry{safety: &OnChainSafety{Severity: 1, Flags: 0, EvidenceHash: params.EvidenceHash}},
			args:    []string{asset.String()},
			wantErr: "severity differs",
			outCode: 4,
		},
		{
			name:    "stale",
			scanner: mockScanner{report: validRep},
			reader:  mockRegistry{safety: &OnChainSafety{Severity: 0, Flags: 0, EvidenceHash: params.EvidenceHash, AttestedAt: uint64(now.Add(-48 * time.Hour).Unix())}},
			args:    []string{"-max-age=24h", asset.String()},
			wantErr: "attestation is stale",
			outCode: 5,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := runVerifyWithDependencies(context.Background(), tc.args, tc.scanner, tc.reader, func() time.Time { return now })
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected success, got %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				if e, ok := err.(interface{ ExitCode() int }); ok {
					if e.ExitCode() != tc.outCode {
						t.Errorf("expected exit code %d, got %d", tc.outCode, e.ExitCode())
					}
				} else {
					t.Errorf("error does not implement ExitCode()")
				}
			}
		})
	}
}
