// Package attest turns a scan report into the exact arguments of an on-chain
// attest() call, and specifies the bytes that evidence_hash commits to.
//
// The registry contract stores a severity number that a caller has to trust.
// evidence_hash is what makes that trust checkable: it is a SHA-256 over a
// canonical encoding of the report, so anyone can re-run the scanner and prove
// an on-chain attestation corresponds to specific evidence rather than to a
// number someone typed. The encoding therefore has to be specified precisely
// enough to reimplement, which is what this package is for.
package attest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/use-assay/assay/internal/mechanics"
)

// PreimageVersion is the first line of every canonical preimage. It is part of
// what gets hashed, so a future encoding change cannot silently produce a hash
// that a verifier would compare against v1 bytes.
const PreimageVersion = "assay-evidence-v1"

// Params is one attest() call: the arguments, and nothing else.
//
// Asset is the classic identifier the scanner read. The contract keys on the
// asset's Stellar Asset Contract address, which is network-scoped and derived
// separately (see docs/deployment.md) — deriving it is not this package's job,
// because it needs a network passphrase and this package needs no I/O at all.
type Params struct {
	Asset        mechanics.Asset `json:"asset"`
	Severity     uint32          `json:"severity"`
	SeverityName string          `json:"severity_name"`
	Flags        uint32          `json:"flags"`
	Mechanics    []string        `json:"mechanics"`
	EvidenceHash string          `json:"evidence_hash"`
	ScannedAt    string          `json:"scanned_at"`
	Preimage     string          `json:"preimage,omitempty"`
}

// ErrInconsistent reports a report whose severity the contract would reject.
var ErrInconsistent = errors.New("attest: attestation violates the confiscation invariant")

// ErrUndetermined reports a scan that could not complete, and so must not be
// written on-chain at all.
var ErrUndetermined = errors.New("attest: scan is undetermined, so there is nothing to attest")

// ErrUnevaluated reports a report whose capability axis was never evaluated:
// it carries mechanics.Unevaluated, not a severity level. This is the specific
// case behind most undetermined reports — the flags were never read — and it
// is refused with its own error so a caller can tell "no capability statement
// exists" apart from "a source was down". Like ErrUndetermined, it means there
// is nothing to attest.
var ErrUnevaluated = errors.New("attest: capability axis was never evaluated, so there is no severity to attest")

// ProvenanceStatus represents the result of evaluating scanner version binding.
type ProvenanceStatus string

const (
	ProvenanceValid   ProvenanceStatus = "valid"
	ProvenanceInvalid ProvenanceStatus = "invalid" // Version below caller's minimum
	ProvenanceUnknown ProvenanceStatus = "unknown" // No version recorded (pre-v2 attestation)
)

// ErrUnknownProvenance reports an attestation produced without version binding (pre-v2).
var ErrUnknownProvenance = errors.New("attest: unknown provenance, attestation has no recorded scanner version")

// ErrScannerDowngrade reports an attestation carrying a scanner version below the caller's minimum acceptable version.
var ErrScannerDowngrade = errors.New("attest: scanner version is below caller's minimum acceptable version")

// VerifyVersion checks whether a recorded scanner version meets a caller's minimum version requirement.
// An absent (empty) recorded version is reported as unknown provenance rather than silently accepted or invalidated.
func VerifyVersion(recordedVersion string, minVersion string) (ProvenanceStatus, error) {
	recorded := strings.TrimSpace(recordedVersion)
	if recorded == "" {
		return ProvenanceUnknown, ErrUnknownProvenance
	}
	if CompareVersions(recorded, minVersion) < 0 {
		return ProvenanceInvalid, fmt.Errorf("%w: recorded %q < minimum %q", ErrScannerDowngrade, recorded, minVersion)
	}
	return ProvenanceValid, nil
}

// CompareVersions compares two semver version strings (e.g. "v1.0.0", "1.0.0", "v0.1.0").
// It returns -1 if v1 < v2, 0 if v1 == v2, and 1 if v1 > v2.
func CompareVersions(v1, v2 string) int {
	clean1 := strings.TrimPrefix(strings.TrimSpace(v1), "v")
	clean2 := strings.TrimPrefix(strings.TrimSpace(v2), "v")

	parts1 := strings.Split(clean1, ".")
	parts2 := strings.Split(clean2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var num1, num2 int
		if i < len(parts1) {
			num1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			num2, _ = strconv.Atoi(parts2[i])
		}
		if num1 < num2 {
			return -1
		}
		if num1 > num2 {
			return 1
		}
	}
	return 0
}


// FromReport derives the attest() arguments for a scan report.
//
// It re-checks the confiscation invariant that the contract enforces at write
// time. Catching it here is not redundant: a submitter should find out that an
// attestation is inconsistent before spending a transaction on it, and a report
// that trips this is a bug in the engine rather than a fee to pay.
func FromReport(rep *mechanics.Report) (Params, error) {
	flags := uint32(rep.Mechanics)
	sev := uint32(rep.Severity)

	// An unevaluated capability axis is refused before the generic undetermined
	// refusal so the canonical case (a Subject whose Stat was never loaded)
	// reports the specific error. The sentinel is deliberately outside the
	// ABI's 0..4 range: the contract would reject it as InvalidSeverity, but
	// catching it here names the actual problem and keeps it out of the
	// preimage entirely.
	if rep.Severity == mechanics.Unevaluated || rep.Base == mechanics.Unevaluated {
		return Params{}, fmt.Errorf("%w: capability was never derived from issuer flags", ErrUnevaluated)
	}

	// A partial scan is refused outright rather than attested with the severity
	// it managed to reach. On-chain there is nowhere to put the caveat: a
	// consumer calling get_safety sees a severity and a timestamp, and has no
	// way to discover that the reputation axis was never read. Writing a level
	// derived from half the evidence would be indistinguishable, to every
	// downstream gate, from one derived from all of it.
	//
	// So the honest options are to attest a complete scan or to attest nothing,
	// and the contract already treats "nothing" correctly — get_safety returns
	// None and every gate fails closed on it.
	if rep.Undetermined {
		return Params{}, fmt.Errorf("%w: %s could not complete",
			ErrUndetermined, strings.Join(rep.UndeterminedChecks, ", "))
	}

	if flags&uint32(mechanics.ConfiscationMask) != 0 && sev < uint32(mechanics.High) {
		return Params{}, fmt.Errorf("%w: clawback bit set at severity %d", ErrInconsistent, sev)
	}

	pre := Preimage(rep)
	sum := sha256.Sum256([]byte(pre))

	return Params{
		Asset:        rep.Asset,
		Severity:     sev,
		SeverityName: rep.Severity.String(),
		Flags:        flags,
		Mechanics:    rep.MechanicNames,
		EvidenceHash: hex.EncodeToString(sum[:]),
		ScannedAt:    rep.ScannedAt.UTC().Format("2006-01-02T15:04:05Z"),
		Preimage:     pre,
	}, nil
}

// Preimage renders the canonical bytes that evidence_hash commits to.
//
// The format is line-oriented, tab-separated, LF-terminated, UTF-8:
//
//	assay-evidence-v1
//	asset	CODE-ISSUER
//	severity	N
//	base_severity	N
//	escalated	true|false
//	mechanics	N
//	accountability	NAME
//	evidence	SOURCE	URL	CLAIM      (one per claim, sorted)
//
// Two decisions in here are worth stating outright.
//
// Retrieval timestamps are excluded. If they were hashed, the same unchanged
// evidence would produce a different hash on every scan, and the field would be
// unverifiable by anyone who was not present for the original fetch. Excluding
// them means the hash commits to what the sources claimed, not to when they
// were asked; the on-chain attested_at carries the time dimension, and a
// verifier re-scans and compares hashes. The cost is that the hash cannot
// distinguish a fresh confirmation from a stale one, which is exactly why
// is_safe takes max_age_secs against attested_at rather than trusting this.
//
// Evidence lines are sorted bytewise rather than left in check order, so
// reordering or adding a check does not change the hash for evidence that did
// not change.
func Preimage(rep *mechanics.Report) string {
	var b strings.Builder

	b.WriteString(PreimageVersion)
	b.WriteByte('\n')
	line(&b, "asset", rep.Asset.String())
	line(&b, "severity", strconv.FormatUint(uint64(rep.Severity), 10))
	line(&b, "base_severity", strconv.FormatUint(uint64(rep.Base), 10))
	line(&b, "escalated", strconv.FormatBool(rep.Escalated))
	line(&b, "mechanics", strconv.FormatUint(uint64(rep.Mechanics), 10))
	line(&b, "accountability", string(rep.Accountability))

	ev := make([]string, 0, len(rep.Evidence))
	for _, e := range rep.Evidence {
		ev = append(ev, "evidence\t"+escape(e.Source)+"\t"+escape(e.URL)+"\t"+escape(e.Claim))
	}
	sort.Strings(ev)
	for _, l := range ev {
		b.WriteString(l)
		b.WriteByte('\n')
	}

	return b.String()
}

// line writes one key/value record.
func line(b *strings.Builder, key, val string) {
	b.WriteString(key)
	b.WriteByte('\t')
	b.WriteString(escape(val))
	b.WriteByte('\n')
}

// escape makes a field unambiguous inside a tab-separated line.
//
// Claims embed third-party text — a directory name, a domain, a toml error —
// so a field can contain anything an issuer chose to publish. Without escaping,
// an issuer could put a tab or a newline in its directory name and forge the
// preimage of a different report. The replacement order matters: backslash
// first, or the escapes introduced below would themselves be escaped.
func escape(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		"\t", `\t`,
		"\n", `\n`,
		"\r", `\r`,
	)
	return r.Replace(s)
}
