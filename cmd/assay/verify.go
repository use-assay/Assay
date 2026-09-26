package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/use-assay/assay/internal/attest"
	"github.com/use-assay/assay/internal/mechanics"
	"github.com/use-assay/assay/internal/scan"
)

const registryAddress = "CBK4FBIHMDTXCUPE4E3ZDVSFJSCY5FJETTKNIQPN4LFJIKKIBLKIXQ73"

type OnChainSafety struct {
	Severity     uint32 `json:"severity"`
	Flags        uint32 `json:"flags"`
	EvidenceHash string `json:"evidence_hash"`
	AttestedAt   uint64 `json:"attested_at"`
}

func (o *OnChainSafety) UnmarshalJSON(b []byte) error {
	type Alias OnChainSafety
	aux := &struct {
		*Alias
		EvidenceHash interface{} `json:"evidence_hash"`
	}{
		Alias: (*Alias)(o),
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	switch v := aux.EvidenceHash.(type) {
	case string:
		o.EvidenceHash = v
	case []interface{}:
		buf := make([]byte, len(v))
		for i, val := range v {
			if num, ok := val.(float64); ok {
				buf[i] = byte(num)
			}
		}
		o.EvidenceHash = hex.EncodeToString(buf)
	}
	return nil
}

type RegistryReader interface {
	Read(ctx context.Context, asset mechanics.Asset) (*OnChainSafety, error)
}

type CLIRegistryReader struct{}

func (c CLIRegistryReader) Read(ctx context.Context, asset mechanics.Asset) (*OnChainSafety, error) {
	assetArg := fmt.Sprintf("%s:%s", asset.Code, asset.Issuer)
	cmd := exec.CommandContext(ctx, "stellar", "contract", "id", "asset", "--asset", assetArg, "--network", "testnet")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("derive SAC: %v", err)
	}
	sac := strings.TrimSpace(string(out))

	cmd = exec.CommandContext(ctx, "stellar", "contract", "invoke", "--id", registryAddress, "--network", "testnet", "--", "get_safety", "--asset", sac)
	out, err = cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("invoke get_safety: %v (output: %s)", err, bytes.TrimSpace(out))
	}
	
	s := strings.TrimSpace(string(out))
	if s == "null" || s == "None" || s == "" {
		return nil, nil
	}
	
	var safety OnChainSafety
	if err := json.Unmarshal([]byte(s), &safety); err != nil {
		return nil, fmt.Errorf("parse on-chain safety %q: %v", s, err)
	}
	return &safety, nil
}

func runVerify(args []string) error {
	return runVerifyWithDependencies(context.Background(), args, scan.New(), CLIRegistryReader{}, time.Now)
}

type Scanner interface {
	Scan(ctx context.Context, asset mechanics.Asset) (*mechanics.Report, error)
}

type verifyResult struct {
	outcome string
	msg     string
}

func (r verifyResult) Error() string { return r.msg }

func (r verifyResult) ExitCode() int {
	switch r.outcome {
	case "unverifiable": return 2
	case "absent":       return 3
	case "mismatch":     return 4
	case "stale":        return 5
	default:             return 1
	}
}

func runVerifyWithDependencies(ctx context.Context, args []string, scanner Scanner, reader RegistryReader, now func() time.Time) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	maxAgeStr := fs.String("max-age", "", "treat attestation older than this duration as failure (e.g. 24h)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("verify takes exactly one asset (CODE-ISSUER)")
	}

	asset, err := scan.ParseAsset(fs.Arg(0))
	if err != nil {
		return err
	}

	var maxAge time.Duration
	if *maxAgeStr != "" {
		maxAge, err = time.ParseDuration(*maxAgeStr)
		if err != nil {
			return fmt.Errorf("invalid max-age: %v", err)
		}
	}

	report, err := scanner.Scan(ctx, asset)
	if err != nil {
		return err
	}

	if report.Undetermined {
		fmt.Printf("scan undetermined: %s could not complete\n", strings.Join(report.UndeterminedChecks, ", "))
		return &verifyResult{"unverifiable", "scan undetermined"}
	}

	params, err := attest.FromReport(report)
	if err != nil {
		// e.g. ErrUnevaluated
		fmt.Printf("scan unverifiable: %v\n", err)
		return &verifyResult{"unverifiable", err.Error()}
	}

	onChain, err := reader.Read(ctx, asset)
	if err != nil {
		return fmt.Errorf("read registry: %v", err)
	}

	if onChain == nil {
		fmt.Println("absent: no attestation found on-chain")
		return &verifyResult{"absent", "no attestation on-chain"}
	}

	if onChain.Severity != params.Severity {
		fmt.Printf("mismatch: severity differs (scan %d, on-chain %d)\n", params.Severity, onChain.Severity)
		return &verifyResult{"mismatch", "severity differs"}
	}
	if onChain.Flags != params.Flags {
		fmt.Printf("mismatch: flags differ (scan %d, on-chain %d)\n", params.Flags, onChain.Flags)
		return &verifyResult{"mismatch", "flags differ"}
	}
	if onChain.EvidenceHash != params.EvidenceHash {
		fmt.Printf("mismatch: evidence_hash differs (scan %s, on-chain %s)\n", params.EvidenceHash, onChain.EvidenceHash)
		return &verifyResult{"mismatch", "evidence_hash differs"}
	}

	attestedAt := time.Unix(int64(onChain.AttestedAt), 0)
	age := now().Sub(attestedAt)
	staleMsg := ""
	if maxAge > 0 && age > maxAge {
		staleMsg = fmt.Sprintf(" (stale, age %s > %s)", age.Round(time.Second), maxAge)
	}

	fmt.Printf("agreement: severity %d, flags %d, hash %s, age %s%s\n", 
		params.Severity, params.Flags, params.EvidenceHash[:8], age.Round(time.Second), staleMsg)

	if maxAge > 0 && age > maxAge {
		return &verifyResult{"stale", "attestation is stale"}
	}

	return nil
}
