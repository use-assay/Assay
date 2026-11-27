// Command refreshfixture re-captures one corpus fixture from the URLs recorded
// in PROVENANCE.md and prints what changed, so a maintainer can judge whether a
// moved verdict reflects a real change or a regression.
//
// It requires network access and is therefore not run by CI. Invoke it through
// the make target:
//
//	make refresh-fixture SUBJECT=aqua-clear-verified
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/use-assay/assay/internal/eval"
	"github.com/use-assay/assay/internal/fixtures"
	"github.com/use-assay/assay/internal/mechanics"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, &fixtures.Refresher{Client: http.DefaultClient}))
}

type refresher interface {
	Refresh(context.Context, string, string, bool) (*fixtures.Result, error)
}

// applier is implemented by a refresher that can write a set of changes it has
// already fetched, which lets the verdict-change guard run between fetching and
// writing without a second round of network requests.
type applier interface {
	Apply(root, subject string, changes []fixtures.Change) error
}

func run(args []string, stdout, stderr io.Writer, r refresher) int {
	fs := flag.NewFlagSet("refreshfixture", flag.ContinueOnError)
	fs.SetOutput(stderr)
	subject := fs.String("subject", "", "fixture directory to re-capture, e.g. aqua-clear-verified")
	root := fs.String("root", "internal/mechanics/testdata", "fixture root holding PROVENANCE.md and manifest.json")
	dryRun := fs.Bool("dry-run", false, "show the diff without writing")
	acceptVerdictChange := fs.Bool("accept-verdict-change", false, "proceed even when re-capturing changes the subject's eval verdict")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *subject == "" {
		_, _ = fmt.Fprintln(stderr, "usage: refresh-fixture -subject <dir> [-root <dir>] [-dry-run] [-accept-verdict-change]")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	ap, canGuard := r.(applier)
	// With an applier the fetch is a preview and the write happens after the
	// verdict guard; without one the refresher fetches and writes in one step.
	res, err := r.Refresh(ctx, *root, *subject, !*dryRun && !canGuard)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "refresh %s: %v\n", *subject, err)
		return 1
	}

	if len(res.Changed) == 0 {
		_, _ = fmt.Fprintf(stdout, "%s: no change; the fixtures match the live sources\n", *subject)
		return 0
	}
	for _, c := range res.Changed {
		_, _ = fmt.Fprint(stdout, fixtures.Unified(c.File, c.Old, c.New))
	}
	if *dryRun {
		_, _ = fmt.Fprintf(stdout, "\n%s: %d file(s) would change (dry run; nothing written)\n", *subject, len(res.Changed))
		return 0
	}

	var oldVerdict, newVerdict string
	if canGuard {
		// Verdict-change guard: run eval on the refreshed fixtures before writing
		// them so a label flip is never silent.
		oldVerdict, newVerdict, err = checkVerdictChange(ctx, *root, *subject, res.Changed)
		if err != nil {
			// Best-effort: a subject outside the eval corpus cannot be loaded, so
			// the guard falls through rather than blocking the refresh.
			_, _ = fmt.Fprintf(stderr, "verdict check skipped (%v); writing without guard\n", err)
		} else if oldVerdict != newVerdict {
			_, _ = fmt.Fprintf(stdout, "\nverdict would change: %s → %s\n", oldVerdict, newVerdict)
			if !*acceptVerdictChange {
				_, _ = fmt.Fprintln(stderr, "re-run with -accept-verdict-change to proceed")
				return 1
			}
			_, _ = fmt.Fprintln(stdout, "verdict change accepted via -accept-verdict-change")
		}
		if err := ap.Apply(*root, *subject, res.Changed); err != nil {
			_, _ = fmt.Fprintf(stderr, "write %s: %v\n", *subject, err)
			return 1
		}
		if err == nil && oldVerdict != newVerdict && oldVerdict != "" {
			if aerr := annotateVerdictChange(*root, *subject, oldVerdict, newVerdict); aerr != nil {
				_, _ = fmt.Fprintf(stderr, "record verdict change in PROVENANCE: %v\n", aerr)
			}
		}
	}

	_, _ = fmt.Fprintf(stdout, "\n%s: %d file(s) updated and capture date refreshed\n", *subject, len(res.Changed))
	return 0
}

// checkVerdictChange writes the refreshed files into a temp directory
// alongside a copy of the existing fixture set, runs the engine on both,
// and returns (oldSeverity, newSeverity). An error means the subject
// could not be loaded (e.g. not in the eval corpus).
func checkVerdictChange(ctx context.Context, root, subject string, changes []fixtures.Change) (string, string, error) {
	engine := mechanics.NewEngine()

	// Old verdict: load from the existing fixture directory.
	old, err := eval.LoadSubject(root, subject)
	if err != nil {
		return "", "", fmt.Errorf("load original subject: %w", err)
	}
	oldRep, err := engine.Run(ctx, old)
	if err != nil {
		return "", "", fmt.Errorf("run engine on original: %w", err)
	}

	// New verdict: overlay the refreshed files onto a temp copy.
	tempDir, err := os.MkdirTemp("", "refreshfixture-*")
	if err != nil {
		return "", "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	subjectTempDir := filepath.Join(tempDir, subject)
	if err := copyDir(filepath.Join(root, subject), subjectTempDir); err != nil {
		return "", "", fmt.Errorf("copy fixture dir: %w", err)
	}
	for _, c := range changes {
		// c.File is relative to root, e.g. "aqua-clear-verified/asset.json"
		rel, err := filepath.Rel(subject, filepath.FromSlash(c.File))
		if err != nil {
			rel = filepath.Base(filepath.FromSlash(c.File))
		}
		dest := filepath.Join(subjectTempDir, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return "", "", err
		}
		if err := os.WriteFile(dest, c.New, 0o644); err != nil {
			return "", "", err
		}
	}

	newSubject, err := eval.LoadSubject(tempDir, subject)
	if err != nil {
		return "", "", fmt.Errorf("load refreshed subject: %w", err)
	}
	newRep, err := engine.Run(ctx, newSubject)
	if err != nil {
		return "", "", fmt.Errorf("run engine on refreshed: %w", err)
	}

	return oldRep.Severity.String(), newRep.Severity.String(), nil
}

// copyDir recursively copies the contents of src to dst.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("relative path of %s: %w", path, err)
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if err := os.WriteFile(target, b, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		return nil
	})
}

// annotateVerdictChange appends a verdict-change note to PROVENANCE.md for
// the subject, so the history of label movements is auditable.
func annotateVerdictChange(root, subject, oldV, newV string) error {
	provPath := filepath.Join(root, "PROVENANCE.md")
	prov, err := os.ReadFile(provPath)
	if err != nil {
		return fmt.Errorf("read provenance: %w", err)
	}
	note := fmt.Sprintf("\n`%s` verdict changed %s → %s on refresh (%s).\n",
		subject, oldV, newV, time.Now().UTC().Format("2006-01-02"))
	if err := os.WriteFile(provPath, append(prov, []byte(note)...), 0o644); err != nil {
		return fmt.Errorf("write provenance: %w", err)
	}
	return nil
}
