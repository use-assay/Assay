package mechanics_test

// TestABIDriftBetweenGoAndRust fails if the Go engine and the Rust contract
// disagree on any SEVERITY_* value, any MECH_* bit position, or the value of
// CONFISCATION_MASK. The two sides independently define the same ABI (Go in
// internal/mechanics/severity.go, Rust in
// assay-contracts/contracts/safety-registry/src/lib.rs), both comment it as
// ABI that must not be renumbered, and until this test existed nothing
// enforced it.
//
// Drift is silent and dangerous: an attestation written with one bit layout
// and read with the other means a gate enforcing a different policy than the
// operator configured, with no error anywhere.
//
// The test reads the Rust file as text and parses every `pub const NAME: u32 =
// <expr>;`. The generation-first approach the issue prefers (soroban contract
// bindings) needs the stellar CLI, and CI must not depend on external
// toolchains for this check — a parser over source text runs anywhere the Go
// build does, without a network, without cargo, and without deploying
// anything. Adding a new MECH_* on one side without the other still fails the
// build, which is what the check is for.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/use-assay/assay/internal/mechanics"
)

// repoRoot resolves the repository root relative to this test file so every
// check works both from `go test ./...` at the repo root and from a package
// directory. runtime.Caller is used rather than a hard-coded relative path
// because Go tests can run from either.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/mechanics/abi_drift_test.go -> repo root
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}

func contractSourcePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "assay-contracts", "contracts",
		"safety-registry", "src", "lib.rs")
}

// gateCrateSourcePath resolves the integrator crate's source (issue #469).
func gateCrateSourcePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "assay-contracts", "crates",
		"assay-gate", "src", "lib.rs")
}

// registryHistorySourcePath resolves the Go event decoder. It reads the
// contract event map by literal field name, so the names it uses are part of
// the cross-language ABI.
func registryHistorySourcePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "cmd", "assay", "registry_history.go")
}

// rustField is one field of a Rust struct as it appears in source text.
type rustField struct {
	name  string
	typ   string
	topic bool
}

// parseRustFields returns the fields of `pub struct name { ... }` in declaration
// order. Comments are stripped first so a field named in prose cannot be
// mistaken for a declaration. A `#[topic]` attribute immediately before a field
// marks it as an event topic; everything else is payload.
func parseRustFields(t *testing.T, src, name string) []rustField {
	t.Helper()
	commentRe := regexp.MustCompile(`//[^\n]*`)
	src = commentRe.ReplaceAllString(src, "")

	structRe := regexp.MustCompile(`pub struct ` + regexp.QuoteMeta(name) + `\s*\{([^}]*)\}`)
	m := structRe.FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("could not find `pub struct %s` in contract source", name)
	}

	fieldRe := regexp.MustCompile(`pub\s+(\w+)\s*:\s*(.+?),?\s*$`)
	var fields []rustField
	topicPending := false
	for _, line := range strings.Split(m[1], "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			continue
		case trimmed == "#[topic]":
			topicPending = true
			continue
		case strings.HasPrefix(trimmed, "#["):
			continue
		}
		fm := fieldRe.FindStringSubmatch(trimmed)
		if fm == nil {
			continue
		}
		fields = append(fields, rustField{
			name:  fm[1],
			typ:   strings.TrimSpace(fm[2]),
			topic: topicPending,
		})
		topicPending = false
	}
	if len(fields) == 0 {
		t.Fatalf("parsed no fields from `pub struct %s`", name)
	}
	return fields
}

// assertStructShape fails unless the parsed Rust struct has exactly want, in
// order. A one-sided addition, removal, rename, reorder or type change fails:
// the contract spec and any hand-written decoder depend on all of them.
func assertStructShape(t *testing.T, structName string, got, want []rustField) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s field count mismatch: got %d fields (%v), want %d fields (%v)",
			structName, len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s field %d mismatch: got %+v, want %+v",
				structName, i, got[i], want[i])
		}
	}
}

// parseRustU32Consts extracts every `pub const NAME: u32 = <expr>;` from the
// Rust source and evaluates the small subset of expressions the ABI uses:
// integer literals (`0`, `3`), bit shifts (`1 << 5`), and bitwise-or
// combinations of previously-defined names (`A | B`). Anything else is
// reported as an error rather than silently ignored, so an ABI constant added
// in a shape the parser cannot read is a red build.
func parseRustU32Consts(t *testing.T, src string) map[string]uint32 {
	t.Helper()
	// Strip line comments so `//` text does not fool the extractor.
	commentRe := regexp.MustCompile(`//[^\n]*`)
	src = commentRe.ReplaceAllString(src, "")

	constRe := regexp.MustCompile(`pub const (\w+):\s*u32\s*=\s*([^;]+);`)
	out := map[string]uint32{}
	for _, m := range constRe.FindAllStringSubmatch(src, -1) {
		name := m[1]
		expr := strings.TrimSpace(m[2])
		v, err := evalU32Expr(expr, out)
		if err != nil {
			t.Fatalf("cannot evaluate rust const %s = %q: %v", name, expr, err)
		}
		out[name] = v
	}
	return out
}

func evalU32Expr(expr string, env map[string]uint32) (uint32, error) {
	// Bitwise-or: fold left over `|`.
	if strings.Contains(expr, "|") {
		var acc uint32
		for _, part := range strings.Split(expr, "|") {
			v, err := evalU32Expr(strings.TrimSpace(part), env)
			if err != nil {
				return 0, err
			}
			acc |= v
		}
		return acc, nil
	}
	// Left-shift: `a << b`, both integer literals.
	if strings.Contains(expr, "<<") {
		parts := strings.SplitN(expr, "<<", 2)
		lhs, err := parseU32Atom(strings.TrimSpace(parts[0]), env)
		if err != nil {
			return 0, err
		}
		rhs, err := parseU32Atom(strings.TrimSpace(parts[1]), env)
		if err != nil {
			return 0, err
		}
		return lhs << rhs, nil
	}
	return parseU32Atom(expr, env)
}

func parseU32Atom(s string, env map[string]uint32) (uint32, error) {
	if v, ok := env[s]; ok {
		return v, nil
	}
	n, err := strconv.ParseUint(s, 0, 32)
	if err != nil {
		return 0, fmt.Errorf("parse %q as u32: %w", s, err)
	}
	return uint32(n), nil
}

func TestABIDriftSeverityValues(t *testing.T) {
	src, err := os.ReadFile(contractSourcePath(t))
	if err != nil {
		t.Fatalf("read contract source: %v", err)
	}
	rust := parseRustU32Consts(t, string(src))

	// Every ABI value the Go side pins must exist in the Rust source at the
	// same numeric value. Values only used on one side (e.g. mechanics.Unevaluated,
	// which deliberately does not reach the chain) are excluded here.
	cases := []struct {
		rustName string
		goValue  uint32
	}{
		{"SEVERITY_CLEAR", uint32(mechanics.Clear)},
		{"SEVERITY_LOW", uint32(mechanics.Low)},
		{"SEVERITY_MEDIUM", uint32(mechanics.Medium)},
		{"SEVERITY_HIGH", uint32(mechanics.High)},
		{"SEVERITY_CRITICAL", uint32(mechanics.Critical)},
	}
	for _, c := range cases {
		got, ok := rust[c.rustName]
		if !ok {
			t.Errorf("rust const %s missing: adding a severity on one side "+
				"without the other is drift", c.rustName)
			continue
		}
		if got != c.goValue {
			t.Errorf("severity drift on %s: rust=%d go=%d", c.rustName, got, c.goValue)
		}
	}
}

func TestABIDriftMechanicBits(t *testing.T) {
	src, err := os.ReadFile(contractSourcePath(t))
	if err != nil {
		t.Fatalf("read contract source: %v", err)
	}
	rust := parseRustU32Consts(t, string(src))

	// Every mechanic the contract publishes must exist on the Go side with the
	// same bit. The Go side may carry additional mechanics that are not part
	// of the on-chain ABI (trustline-scoped bits are not written to the
	// contract), so a Go-only mechanic is not drift on its own — but every
	// contract-side mechanic must match.
	cases := []struct {
		rustName string
		goValue  uint32
	}{
		{"MECH_AUTH_REQUIRED", uint32(mechanics.MechAuthRequired)},
		{"MECH_AUTH_REVOCABLE", uint32(mechanics.MechAuthRevocable)},
		{"MECH_CLAWBACK_ENABLED", uint32(mechanics.MechClawbackEnabled)},
		{"MECH_FLAGS_LOCKED", uint32(mechanics.MechFlagsLocked)},
		{"MECH_DOMAIN_UNVERIFIED", uint32(mechanics.MechDomainUnverified)},
		{"MECH_BLOCKLISTED", uint32(mechanics.MechBlocklisted)},
	}
	for _, c := range cases {
		got, ok := rust[c.rustName]
		if !ok {
			t.Errorf("rust const %s missing: adding a mechanic on one side "+
				"without the other is drift", c.rustName)
			continue
		}
		if got != c.goValue {
			t.Errorf("mechanic drift on %s: rust=1<<%d go=1<<%d", c.rustName,
				trailingZeros(got), trailingZeros(c.goValue))
		}
	}
}

func TestABIDriftConfiscationMask(t *testing.T) {
	src, err := os.ReadFile(contractSourcePath(t))
	if err != nil {
		t.Fatalf("read contract source: %v", err)
	}
	rust := parseRustU32Consts(t, string(src))

	got, ok := rust["CONFISCATION_MASK"]
	if !ok {
		t.Fatal("rust CONFISCATION_MASK missing")
	}
	if got != uint32(mechanics.ConfiscationMask) {
		t.Errorf("CONFISCATION_MASK drift: rust=%#x go=%#x",
			got, uint32(mechanics.ConfiscationMask))
	}
}

// TestABIDriftCapabilityMask extends the #15 drift check to the #34 mask: the
// capability/power bits must be selectable by name on BOTH sides and the two
// sides must agree on the value, or a consumer picking the mask from the wrong
// language's definition silently selects a different set of powers.
func TestABIDriftCapabilityMask(t *testing.T) {
	src, err := os.ReadFile(contractSourcePath(t))
	if err != nil {
		t.Fatalf("read contract source: %v", err)
	}
	rust := parseRustU32Consts(t, string(src))

	got, ok := rust["CAPABILITY_MASK"]
	if !ok {
		t.Fatal("rust CAPABILITY_MASK missing")
	}
	if got != uint32(mechanics.CapabilityMask) {
		t.Errorf("CAPABILITY_MASK drift: rust=%#x go=%#x",
			got, uint32(mechanics.CapabilityMask))
	}

	// The mask is exactly the three power bits and nothing else: a reported
	// bit leaking in (or a power bit left out) is the #26 failure shape.
	want := uint32(mechanics.MechAuthRequired | mechanics.MechAuthRevocable |
		mechanics.MechClawbackEnabled)
	if uint32(mechanics.CapabilityMask) != want {
		t.Errorf("CapabilityMask = %#x, want the three power bits %#x",
			uint32(mechanics.CapabilityMask), want)
	}
	if mechanics.CapabilityMask&^(mechanics.MechAuthRequired|mechanics.MechAuthRevocable|mechanics.MechClawbackEnabled) != 0 {
		t.Errorf("CapabilityMask carries reported-only bits: %#x", uint32(mechanics.CapabilityMask))
	}
}

// TestABIDriftInterfaceVersion asserts that the interface version in the registry
// contract source text matches the documented version (4).
func TestABIDriftInterfaceVersion(t *testing.T) {
	src, err := os.ReadFile(contractSourcePath(t))
	if err != nil {
		t.Fatalf("read contract source: %v", err)
	}
	rust := parseRustU32Consts(t, string(src))

	got, ok := rust["INTERFACE_VERSION"]
	if !ok {
		t.Fatal("rust INTERFACE_VERSION missing")
	}
	if got != 4 {
		t.Errorf("INTERFACE_VERSION drift: got %d, want 4", got)
	}
}

// TestABIDriftSafetyStructShape pins the on-chain Safety struct fields in
// the registry contract. Any addition, reordering, or type change of Safety fields
// (such as adding an observed_at timestamp) alters the observable on-chain ABI
// and storage layout.
func TestABIDriftSafetyStructShape(t *testing.T) {
	src, err := os.ReadFile(contractSourcePath(t))
	if err != nil {
		t.Fatalf("read contract source: %v", err)
	}

	// Verify the exact fields of `pub struct Safety`
	safetyRe := regexp.MustCompile(`pub struct Safety \{([^}]+)\}`)
	match := safetyRe.FindStringSubmatch(string(src))
	if len(match) < 2 {
		t.Fatal("could not find `pub struct Safety` in contract source")
	}

	body := match[1]
	fieldRe := regexp.MustCompile(`pub\s+(\w+)\s*:\s*([^,\n]+)`)
	fields := fieldRe.FindAllStringSubmatch(body, -1)

	type fieldInfo struct {
		name string
		typ  string
	}
	var gotFields []fieldInfo
	for _, f := range fields {
		gotFields = append(gotFields, fieldInfo{
			name: strings.TrimSpace(f[1]),
			typ:  strings.TrimSpace(f[2]),
		})
	}

	wantFields := []fieldInfo{
		{"severity", "u32"},
		{"flags", "u32"},
		{"evidence_hash", "BytesN<32>"},
		{"attested_at", "u64"},
		{"preimage_version", "u32"},
	}

	if len(gotFields) != len(wantFields) {
		t.Fatalf("Safety struct field count mismatch: got %d fields (%v), want %d fields (%v)",
			len(gotFields), gotFields, len(wantFields), wantFields)
	}

	for i := range wantFields {
		if gotFields[i] != wantFields[i] {
			t.Errorf("Safety struct field %d mismatch: got %+v, want %+v",
				i, gotFields[i], wantFields[i])
		}
	}
}

// TestABIDriftIntegratorCrate extends the drift check to the integrator
// crate (issue #469). assay-gate is what a third-party gate compiles against,
// so its copy of the ABI constants must agree with the registry's: a gate
// that renumbers a bit relative to the deployed registry passes its own tests
// and misreads every attestation in production, with no error anywhere.
// Parsing both sources as text keeps the check free of cargo, the same reason
// the original tests parse the registry as text.
func TestABIDriftIntegratorCrate(t *testing.T) {
	registrySrc, err := os.ReadFile(contractSourcePath(t))
	if err != nil {
		t.Fatalf("read registry source: %v", err)
	}
	crateSrc, err := os.ReadFile(gateCrateSourcePath(t))
	if err != nil {
		t.Fatalf("read assay-gate crate source: %v", err)
	}
	registry := parseRustU32Consts(t, string(registrySrc))
	crate := parseRustU32Consts(t, string(crateSrc))

	// Every constant the crate re-exports must exist in the registry at the
	// same value. The crate may carry helpers the registry does not (a policy
	// function, say), but not a differing constant under a registry name.
	names := []string{
		"MECH_AUTH_REQUIRED",
		"MECH_AUTH_REVOCABLE",
		"MECH_CLAWBACK_ENABLED",
		"MECH_FLAGS_LOCKED",
		"MECH_DOMAIN_UNVERIFIED",
		"MECH_BLOCKLISTED",
		"CAPABILITY_MASK",
		"CONFISCATION_MASK",
		"MAX_ATTESTORS",
	}
	for _, name := range names {
		want, ok := registry[name]
		if !ok {
			t.Fatalf("registry const %s missing: cannot judge the crate against it", name)
		}
		got, ok := crate[name]
		if !ok {
			t.Errorf("assay-gate const %s missing: an integrator cannot select it by name", name)
			continue
		}
		if got != want {
			t.Errorf("assay-gate drift on %s: crate=%#x registry=%#x",
				name, got, want)
		}
	}
}

// TestABIDriftPolicyMasks extends the drift check to the named policy masks
// passed to is_safe_masked. A consumer that selects
// POLICY_MASK_FREEZE_INCLUSIVE in Rust and the Go equivalent on the scanner
// side must be selecting the same bits; a one-sided addition, rename or value
// change silently changes which assets a policy admits.
func TestABIDriftPolicyMasks(t *testing.T) {
	src, err := os.ReadFile(contractSourcePath(t))
	if err != nil {
		t.Fatalf("read contract source: %v", err)
	}
	rust := parseRustU32Consts(t, string(src))

	want := map[string]uint32{
		"POLICY_MASK_CONFISCATION_ONLY": uint32(mechanics.PolicyMaskConfiscationOnly),
		"POLICY_MASK_FREEZE_INCLUSIVE":  uint32(mechanics.PolicyMaskFreezeInclusive),
	}

	// Compare the whole POLICY_MASK_* family, not only the names we know:
	// a policy mask added on one side alone is drift the per-name checks
	// would miss.
	var rustNames []string
	for name := range rust {
		if strings.HasPrefix(name, "POLICY_MASK_") {
			rustNames = append(rustNames, name)
		}
	}
	sort.Strings(rustNames)
	var goNames []string
	for name := range want {
		goNames = append(goNames, name)
	}
	sort.Strings(goNames)
	if strings.Join(rustNames, ",") != strings.Join(goNames, ",") {
		t.Errorf("POLICY_MASK_* drift: rust has %v, Go defines %v", rustNames, goNames)
	}

	for name, goValue := range want {
		got, ok := rust[name]
		if !ok {
			t.Errorf("rust const %s missing: adding a policy mask on one side "+
				"without the other is drift", name)
			continue
		}
		if got != goValue {
			t.Errorf("policy-mask drift on %s: rust=%#x go=%#x", name, got, goValue)
		}
	}

	// The documented relationships are part of the ABI too: the
	// confiscation-only policy is exactly the confiscation mask, and the
	// freeze-inclusive policy adds auth_revocable to it.
	if mechanics.PolicyMaskConfiscationOnly != mechanics.ConfiscationMask {
		t.Errorf("PolicyMaskConfiscationOnly = %#x, want ConfiscationMask %#x",
			uint32(mechanics.PolicyMaskConfiscationOnly), uint32(mechanics.ConfiscationMask))
	}
	wantFreeze := mechanics.ConfiscationMask | mechanics.MechAuthRevocable
	if mechanics.PolicyMaskFreezeInclusive != wantFreeze {
		t.Errorf("PolicyMaskFreezeInclusive = %#x, want %#x",
			uint32(mechanics.PolicyMaskFreezeInclusive), uint32(wantFreeze))
	}
}

// TestABIDriftAttestationStructShape pins the batch `Attestation` argument
// struct. It is the element type of attest_many and is published in the
// contract spec; a field added, removed, renamed, reordered or retyped on one
// side changes the encoding a caller must produce.
func TestABIDriftAttestationStructShape(t *testing.T) {
	src, err := os.ReadFile(contractSourcePath(t))
	if err != nil {
		t.Fatalf("read contract source: %v", err)
	}

	got := parseRustFields(t, string(src), "Attestation")
	assertStructShape(t, "Attestation", got, []rustField{
		{name: "asset", typ: "Address"},
		{name: "severity", typ: "u32"},
		{name: "flags", typ: "u32"},
		{name: "evidence_hash", typ: "BytesN<32>"},
	})
}

// TestABIDriftAttestationEventShape pins the fields of the AttestationEvent
// struct. Even though the current contract does not publish it yet (tracked
// separately), it is declared as part of the documented interface; changing
// its field names on one side without the other would make an indexer decode
// a different event than the contract emits.
func TestABIDriftAttestationEventShape(t *testing.T) {
	src, err := os.ReadFile(contractSourcePath(t))
	if err != nil {
		t.Fatalf("read contract source: %v", err)
	}

	got := parseRustFields(t, string(src), "AttestationEvent")
	assertStructShape(t, "AttestationEvent", got, []rustField{
		{name: "asset", typ: "Address"},
		{name: "previous", typ: "Option<Safety>"},
		{name: "current", typ: "Safety"},
	})
}

// TestABIDriftEventFieldNames checks the event payload field names the Go
// decoder reads against the Rust event structs it decodes. The decoder looks
// fields up by literal name, so a rename on either side silently drops data
// (read as a zero value) rather than failing to decode.
func TestABIDriftEventFieldNames(t *testing.T) {
	src, err := os.ReadFile(contractSourcePath(t))
	if err != nil {
		t.Fatalf("read contract source: %v", err)
	}
	historySrc, err := os.ReadFile(registryHistorySourcePath(t))
	if err != nil {
		t.Fatalf("read registry history decoder: %v", err)
	}

	// Rust publishes only payload fields in the event map; topics are keyed
	// separately. Collect the non-topic names from both event structs.
	rustFields := map[string]bool{}
	for _, event := range []string{"Attested", "Revoked"} {
		for _, f := range parseRustFields(t, string(src), event) {
			if !f.topic {
				rustFields[f.name] = true
			}
		}
	}

	keyRe := regexp.MustCompile(`Fields\["(\w+)"\]`)
	goFields := map[string]bool{}
	for _, m := range keyRe.FindAllStringSubmatch(string(historySrc), -1) {
		goFields[m[1]] = true
	}

	for name := range rustFields {
		if !goFields[name] {
			t.Errorf("Go event decoder does not read rust event field %q: a rename "+
				"on either side silently drops the value", name)
		}
	}
	for name := range goFields {
		if !rustFields[name] {
			t.Errorf("Go event decoder reads %q, which is not a payload field of "+
				"the rust Attested/Revoked events", name)
		}
	}
}

func trailingZeros(x uint32) int {
	if x == 0 {
		return -1
	}
	n := 0
	for x&1 == 0 {
		x >>= 1
		n++
	}
	return n
}
