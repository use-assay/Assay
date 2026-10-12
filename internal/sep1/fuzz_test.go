package sep1_test

// Parse decodes a stellar.toml fetched from a domain the issuer controls. That
// document is attacker-controlled input, and a crash there takes the scanner
// down for every asset, so the fuzz target below feeds it arbitrary bytes.
// Rejected input must come back as an error with no document; accepted input
// must survive a render-and-reparse round trip.

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/use-assay/assay/internal/sep1"
)

// mechanicsTestdataDir resolves internal/mechanics/testdata relative to this
// test file, so the seeds load whether `go test` runs from the repo root or
// from the package directory.
func mechanicsTestdataDir(tb testing.TB) string {
	tb.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		tb.Fatal("runtime.Caller failed")
	}
	// internal/sep1/fuzz_test.go -> internal/mechanics/testdata
	return filepath.Join(filepath.Dir(thisFile), "..", "mechanics", "testdata")
}

// fixtureTomls reads every captured stellar.toml under
// internal/mechanics/testdata. These are documents the scanner actually
// consumed, so they seed the fuzzer with realistic shapes rather than only
// hand-written ones. Failing when none are found keeps the corpus from
// silently going empty after a fixture move.
func fixtureTomls(tb testing.TB) [][]byte {
	tb.Helper()
	matches, err := filepath.Glob(filepath.Join(mechanicsTestdataDir(tb), "*", "stellar.toml"))
	if err != nil {
		tb.Fatalf("glob fixtures: %v", err)
	}
	if len(matches) == 0 {
		tb.Fatal("no fixture stellar.toml files found; the seed corpus would be empty")
	}
	var out [][]byte
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			tb.Fatalf("read %s: %v", path, err)
		}
		out = append(out, raw)
	}
	return out
}

// FuzzParseToml asserts the two states Parse can be in. A rejected input comes
// back as an error with a nil document, never a partially-populated one; an
// accepted input re-renders to TOML and parses back to the same currencies.
func FuzzParseToml(f *testing.F) {
	// Seed corpus: the documents from TestClaims and TestLinkedCurrencies,
	// plus every captured stellar.toml fixture.
	f.Add([]byte(`
[[CURRENCIES]]
code = "USDC"
issuer = "` + issuer + `"

[[CURRENCIES]]
code = "EURC"
issuer = "GDHU6WRG4IEQXM5NZ4BMPKOXHW76MZM4Y2IEMFDVXBSDP6SJY4ITNPP2"
`))
	f.Add([]byte(`
[[CURRENCIES]]
toml = "https://example.com/.well-known/USDC.toml"

[[CURRENCIES]]
code = "AQUA"
issuer = "` + issuer + `"
`))
	for _, b := range fixtureTomls(f) {
		f.Add(b)
	}

	f.Fuzz(func(t *testing.T, b []byte) {
		doc, err := sep1.Parse(b)
		if err != nil {
			// A rejected input must not leave a partial document behind.
			if doc != nil {
				t.Fatalf("Parse(%q) rejected the input but returned %+v", b, doc)
			}
			return
		}
		if doc == nil {
			t.Fatalf("Parse(%q) accepted the input but returned a nil document", b)
		}

		// Accepted input must round-trip: rendering the parsed subset and
		// parsing it again yields the same currencies. Only Currencies is
		// compared because URL and FetchedAt are set by Fetch, not Parse, and
		// the fields the parser deliberately ignores do not survive a render.
		encoded, err := toml.Marshal(doc)
		if err != nil {
			t.Fatalf("Parse(%q) accepted a document that will not marshal: %v", b, err)
		}
		rt, err := sep1.Parse(encoded)
		if err != nil {
			t.Fatalf("Parse(%q): the parsed document does not re-parse: %v (marshalled: %q)", b, err, encoded)
		}
		if !slices.Equal(doc.Currencies, rt.Currencies) {
			t.Fatalf("Parse(%q) did not round-trip:\n before: %+v\n after:  %+v\n marshalled: %q",
				b, doc.Currencies, rt.Currencies, encoded)
		}
	})
}
