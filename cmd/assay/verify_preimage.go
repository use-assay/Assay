package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/use-assay/assay/internal/attest"
)

const (
	verifyMatch           = "MATCH"
	verifyMismatch        = "MISMATCH"
	verifyEncodingDiffers = "ENCODING_DIFFERS"
)

type commandExitError struct {
	code int
	err  error
}

func (e *commandExitError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return ""
}

func runVerifyPreimage(args []string) error {
	fs := flag.NewFlagSet("verify-preimage", flag.ContinueOnError)
	expectedVersion := fs.String("version", "", "expected preimage encoding version")
	if err := fs.Parse(args); err != nil {
		return &commandExitError{code: 3, err: err}
	}
	if fs.NArg() != 2 {
		return &commandExitError{code: 3, err: fmt.Errorf("verify-preimage takes a hash and preimage path")}
	}

	preimage, err := readPreimage(fs.Arg(1))
	if err != nil {
		return &commandExitError{code: 3, err: err}
	}
	outcome, err := verifyPreimage(fs.Arg(0), preimage, *expectedVersion)
	if err != nil {
		return &commandExitError{code: 3, err: err}
	}
	if _, err := fmt.Fprintln(os.Stdout, outcome); err != nil {
		return &commandExitError{code: 3, err: fmt.Errorf("write verification outcome: %w", err)}
	}
	switch outcome {
	case verifyMatch:
		return nil
	case verifyMismatch:
		return &commandExitError{code: 1}
	case verifyEncodingDiffers:
		return &commandExitError{code: 2}
	default:
		return &commandExitError{code: 3, err: fmt.Errorf("unknown verification outcome %q", outcome)}
	}
}

func readPreimage(path string) (string, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return "", fmt.Errorf("read preimage: %w", err)
	}
	return string(data), nil
}

func verifyPreimage(storedHash, preimage, expectedVersion string) (string, error) {
	storedHash = strings.TrimSpace(storedHash)
	decoded, err := hex.DecodeString(storedHash)
	if err != nil || len(decoded) != sha256.Size {
		return "", fmt.Errorf("hash must be 64 hexadecimal characters")
	}
	storedHash = strings.ToLower(storedHash)

	versions := []string{
		attest.PreimageVersion,
		attest.PreimageVersionCheckSet,
		attest.PreimageVersionNetwork,
		attest.PreimageVersionScanner,
	}
	if expectedVersion != "" {
		requestedVersion := expectedVersion
		expectedVersion, err = attest.NormalizePreimageVersion(requestedVersion)
		if err != nil {
			return "", fmt.Errorf("unsupported encoding version %q", requestedVersion)
		}
		versions = []string{expectedVersion}
	}

	actualVersion := strings.SplitN(preimage, "\n", 2)[0]
	currentMatches := hashPreimage(preimage) == storedHash
	if expectedVersion == "" {
		if currentMatches {
			return verifyMatch, nil
		}
	} else if normalizedActualVersion, err := attest.NormalizePreimageVersion(actualVersion); err == nil && normalizedActualVersion == expectedVersion {
		if currentMatches {
			return verifyMatch, nil
		}
		return verifyMismatch, nil
	}

	for _, version := range versions {
		candidate, ok := preimageForVersion(preimage, version)
		if ok && candidate != preimage && hashPreimage(candidate) == storedHash {
			return verifyEncodingDiffers, nil
		}
	}
	if expectedVersion != "" && currentMatches {
		return verifyEncodingDiffers, nil
	}
	return verifyMismatch, nil
}

func preimageForVersion(preimage, version string) (string, bool) {
	lines := strings.Split(preimage, "\n")
	if len(lines) < 2 {
		return "", false
	}
	actualVersion, err := attest.NormalizePreimageVersion(lines[0])
	if err != nil || actualVersion != lines[0] {
		return "", false
	}
	targetVersion, err := attest.NormalizePreimageVersion(version)
	if err != nil || preimageVersionRank(targetVersion) > preimageVersionRank(actualVersion) {
		return "", false
	}

	actualRank := preimageVersionRank(actualVersion)
	targetRank := preimageVersionRank(targetVersion)
	seenChecks, seenNetwork, seenScanner := false, false, false
	filtered := make([]string, 1, len(lines))
	filtered[0] = targetVersion
	for _, line := range lines[1:] {
		switch {
		case strings.HasPrefix(line, "scanner\t"):
			if seenScanner || actualRank < 4 {
				return "", false
			}
			seenScanner = true
			if targetRank >= 4 {
				filtered = append(filtered, line)
			}
		case strings.HasPrefix(line, "checks\t"):
			if seenChecks || actualRank < 2 {
				return "", false
			}
			seenChecks = true
			if targetRank >= 2 {
				filtered = append(filtered, line)
			}
		case strings.HasPrefix(line, "network\t"):
			if seenNetwork || actualRank < 3 {
				return "", false
			}
			seenNetwork = true
			if targetRank >= 3 {
				filtered = append(filtered, line)
			}
		default:
			filtered = append(filtered, line)
		}
	}
	if (actualRank >= 2) != seenChecks || (actualRank >= 3) != seenNetwork || (actualRank >= 4) != seenScanner {
		return "", false
	}
	return strings.Join(filtered, "\n"), actualVersion != targetVersion
}

func preimageVersionRank(version string) int {
	switch version {
	case attest.PreimageVersion:
		return 1
	case attest.PreimageVersionCheckSet:
		return 2
	case attest.PreimageVersionNetwork:
		return 3
	case attest.PreimageVersionScanner:
		return 4
	default:
		return 0
	}
}

func hashPreimage(preimage string) string {
	sum := sha256.Sum256([]byte(preimage))
	return hex.EncodeToString(sum[:])
}
