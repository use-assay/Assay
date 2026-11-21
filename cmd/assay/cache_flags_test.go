package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/use-assay/assay/internal/mechanics"
	"github.com/use-assay/assay/internal/scan"
)

// usageFlagRE finds a flag token in the usage text and, when the next token is
// an upper-case placeholder, records that the flag takes a value.
var (
	usageFlagRE = regexp.MustCompile(`-([a-z][a-z0-9-]*)(?:\s+([A-Z][^\s\]]*))?`)
	// A synopsis line carries its description after a wide gutter. Only the
	// left part names flags; the description may contain hyphens ("on-chain").
	usageGutterRE = regexp.MustCompile(`\s{3,}`)
)

// TestUsageFlagsAreRegistered parses the flags each subcommand's usage section
// names and checks the command registers every one of them. Documentation and
// registration drifted apart once (#566); this test makes the next drift red.
func TestUsageFlagsAreRegistered(t *testing.T) {
	var text strings.Builder
	usage(&text)

	scanningCommands := []string{"scan", "attestation", "history", "verify", "serve"}
	flags := map[string]map[string]bool{}

	add := func(command, name string, takesValue bool) {
		if flags[command] == nil {
			flags[command] = map[string]bool{}
		}
		flags[command][name] = takesValue
	}

	mode := ""
	for _, line := range strings.Split(text.String(), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "Commands that scan also accept:"):
			mode = "scan-common"
			continue
		case strings.HasPrefix(trimmed, "The scan command additionally accepts:"):
			mode = "scan"
			continue
		case strings.HasPrefix(trimmed, "The attestation command additionally accepts:"):
			mode = "attestation"
			continue
		}

		if trimmed == "" {
			mode = ""
			continue
		}

		// A command synopsis line names the flags it takes in brackets.
		if strings.HasPrefix(trimmed, "assay ") {
			fields := strings.Fields(trimmed)
			if len(fields) < 2 {
				continue
			}
			command := fields[1]
			// Keep only the command and its flags: the description to the right
			// of the gutter is prose, not a flag list.
			head := usageGutterRE.Split(trimmed, 2)[0]
			// Strip the command name so a hyphenated name such as
			// registry-history is not read as a -history flag.
			rest := strings.TrimPrefix(head, "assay "+command)
			for _, m := range usageFlagRE.FindAllStringSubmatch(rest, -1) {
				add(command, m[1], m[2] != "")
			}
			continue
		}

		if mode == "" || !strings.HasPrefix(trimmed, "-") {
			continue
		}
		m := usageFlagRE.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		switch mode {
		case "scan-common":
			for _, command := range scanningCommands {
				add(command, m[1], m[2] != "")
			}
		default:
			add(mode, m[1], m[2] != "")
		}
	}

	for command, names := range flags {
		for name, takesValue := range names {
			args := []string{"-" + name}
			if takesValue {
				args = append(args, flagTestValue(name))
			}
			// -h stops parsing once the flag is accepted. An undefined flag
			// fails before it, which is exactly the drift this test catches.
			args = append(args, "-h")
			err := runFlagParse(command, args)
			if !isHelp(err) {
				t.Errorf("%s: flag -%s is named in usage() but not registered: %v", command, name, err)
			}
		}
	}
}

// runFlagParse exercises one command's flag parsing. Passing -h after the
// flags stops parsing once they are accepted; an undefined flag fails first.
func runFlagParse(command string, args []string) error {
	if command == "serve" {
		return runServe(args, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}
	return runWith(append([]string{command}, args...), commandDeps{stdout: io.Discard, stderr: io.Discard})
}

func isHelp(err error) bool {
	return err != nil && (errors.Is(err, flag.ErrHelp) || strings.Contains(err.Error(), flag.ErrHelp.Error()))
}

// TestCachingDocExamplesParse holds the examples in docs/caching.md to the
// flags the binary registers. The flags are parsed without the asset operand,
// which is where parsing would stop, so this is the same parse the example
// performs.
func TestCachingDocExamplesParse(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	docPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "docs", "caching.md")
	doc, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read %s: %v", docPath, err)
	}

	examples := 0
	for _, line := range strings.Split(string(doc), "\n") {
		line = strings.SplitN(line, "#", 2)[0]
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] != "assay" {
			continue
		}
		examples++
		command := fields[1]
		args := []string{}
		for i := 2; i < len(fields); i++ {
			if !strings.HasPrefix(fields[i], "-") {
				continue
			}
			args = append(args, fields[i])
			// The final token is the asset operand, not a flag value.
			if i+1 < len(fields)-1 && !strings.HasPrefix(fields[i+1], "-") {
				args = append(args, fields[i+1])
				i++
			}
		}
		args = append(args, "-h")
		if err := runFlagParse(command, args); !isHelp(err) {
			t.Errorf("example %q does not parse: %v", line, err)
		}
	}
	if examples == 0 {
		t.Fatal("no assay examples found in docs/caching.md")
	}
}

// flagTestValue returns a value that parses for the flag's type. Only durations
// and integers are validated by the flag package; every other value is read as
// a string.
func flagTestValue(name string) string {
	switch {
	case strings.Contains(name, "ttl"):
		return "1s"
	case name == "history-retention", name == "max-age":
		return "1"
	default:
		return "x"
	}
}

// cacheRoundTripFunc adapts a function to http.RoundTripper, so the test can
// answer the stellar.toml fetch without a network.
type cacheRoundTripFunc func(*http.Request) (*http.Response, error)

func (f cacheRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// cacheFixture serves the four responses a scan of an issuer with a
// home_domain makes, and counts the two StellarExpert lookups separately.
type cacheFixture struct {
	horizon   *httptest.Server
	expert    *httptest.Server
	mu        sync.Mutex
	directory int
	blocklist int
}

func newCacheFixture(t *testing.T) *cacheFixture {
	t.Helper()
	f := &cacheFixture{}

	f.horizon = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/assets":
			_, _ = w.Write([]byte(`{"_embedded":{"records":[{"asset_type":"credit_alphanum4","asset_code":"USDC","asset_issuer":"` + cliIssuer + `","flags":{"auth_required":false,"auth_revocable":false,"auth_immutable":false,"auth_clawback_enabled":false}}]}}`))
		case "/accounts/" + cliIssuer:
			_, _ = w.Write([]byte(`{"account_id":"` + cliIssuer + `","home_domain":"example.com","flags":{"auth_required":false,"auth_revocable":false,"auth_immutable":false,"auth_clawback_enabled":false}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.horizon.Close)

	f.expert = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/explorer/directory/blocked-domains/"):
			f.mu.Lock()
			f.blocklist++
			f.mu.Unlock()
			_, _ = w.Write([]byte(`{"domain":"example.com","blocked":false}`))
		case strings.HasPrefix(r.URL.Path, "/explorer/directory/"):
			f.mu.Lock()
			f.directory++
			f.mu.Unlock()
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.expert.Close)

	return f
}

func (f *cacheFixture) counts() (directory, blocklist int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.directory, f.blocklist
}

// cacheDeps builds deps whose scanner points at the fixture. The scanner is
// built once so a second scan sees whatever the first one cached.
func cacheDeps() commandDeps {
	var sc *scan.Scanner
	d := commandDeps{stdout: io.Discard, stderr: io.Discard}
	d.scanner = func(lists []string, opts scan.Options) *scan.Scanner {
		if sc == nil {
			sc = newScannerWithOptions(lists, opts)
			sc.Toml.HTTP = &http.Client{Transport: cacheRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("[[CURRENCIES]]\n")),
					Request:    req,
				}, nil
			})}
		}
		return sc
	}
	d.scan = func(ctx context.Context, a mechanics.Asset) (*mechanics.Report, error) {
		return d.scanner(nil, scan.DefaultOptions()).Scan(ctx, a)
	}
	return d
}

// TestCacheFlagsRefetchCuratedSources proves the flags reach the scanner: with
// -no-cache or a zero TTL, two scans fetch the affected source twice instead of
// reusing the first answer.
func TestCacheFlagsRefetchCuratedSources(t *testing.T) {
	asset := "USDC-" + cliIssuer

	for _, tc := range []struct {
		name      string
		args      []string
		wantDir   int
		wantBlock int
	}{
		{name: "default caches both", wantDir: 1, wantBlock: 1},
		{name: "no-cache", args: []string{"-no-cache"}, wantDir: 2, wantBlock: 2},
		{name: "both TTLs zero", args: []string{"-cache-directory-ttl", "0", "-cache-blocklist-ttl", "0"}, wantDir: 2, wantBlock: 2},
		{name: "directory TTL zero", args: []string{"-cache-directory-ttl", "0"}, wantDir: 2, wantBlock: 1},
		{name: "blocklist TTL zero", args: []string{"-cache-blocklist-ttl", "0"}, wantDir: 1, wantBlock: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newCacheFixture(t)
			t.Setenv("ASSAY_HORIZON_URL", fixture.horizon.URL)
			t.Setenv("ASSAY_STELLAREXPERT_URL", fixture.expert.URL)

			d := cacheDeps()
			for i := 0; i < 2; i++ {
				args := append([]string{"scan"}, tc.args...)
				args = append(args, asset)
				if err := runWith(args, d); err != nil {
					t.Fatalf("scan %d: %v", i+1, err)
				}
			}

			directory, blocklist := fixture.counts()
			if directory != tc.wantDir {
				t.Errorf("directory requests = %d, want %d", directory, tc.wantDir)
			}
			if blocklist != tc.wantBlock {
				t.Errorf("blocklist requests = %d, want %d", blocklist, tc.wantBlock)
			}
		})
	}
}
