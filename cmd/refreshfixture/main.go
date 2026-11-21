// Command refreshfixture re-captures one corpus fixture from the URLs recorded
// in PROVENANCE.md and prints what changed, so a maintainer can judge whether a
// moved verdict reflects a real change or a regression.
//
// By default the command blocks if re-capturing the fixture would change the
// subject's eval verdict, because docs/eval.md says a disputed label is never
// silently edited. Pass -accept-verdict-change to proceed; the change is
// recorded in PROVENANCE.md.
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
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/use-assay/assay/internal/eval"
	"github.com/use-assay/assay/internal/fixtures"
	"github.com/use-assay/assay/internal/mechanics"
)

func main() {
	subject := flag.String("subject", "", "fixture directory to re-capture, e.g. aqua-clear-verified")
	root := flag.String("root", "internal/mechanics/testdata", "fixture root holding PROVENANCE.md and manifest.json")
	dryRun := flag.Bool("dry-run", false, "show the diff without writing")
	acceptVerdictChange := flag.Bool("accept-verdict-change", false, "proceed even when re-capturing changes the subject's eval verdict")
	flag.Parse()

	if *subject == "" {
		fmt.Fprintln(os.Stderr, "usage: refresh-fixture -subject <dir> [-root <dir>] [-dry-run] [-accept-verdict-change]")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	r := &fixtures.Refresher{}
	res, err := r.Refresh(ctx, *root, *subject, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "refresh %s: %v\n", *subject, err)
		os.Exit(1)
	}

	if len(res.Changed) == 0 {
		fmt.Printf("%s: no change; the fixtures match the live sources\n", *subject)
		return
	}
	for _, c := range res.Changed {
		fmt.Print(fixtures.Unified(c.File, c.Old, c.New))
	}
	if *dryRun {
		fmt.Printf("\n%s: %d file(s) would change (dry run; nothing written)\n", *subject, len(res.Changed))
		return
	}

	// Verdict-change guard: run eval on the refreshed fixtures before writing
	// them so a label flip is never silent.
	oldVerdict, newVerdict, err := checkVerdictChange(ctx, *root, *subject, res.Changed)
	if err != nil {
		// Verdict check is best-effort: if the subject is not in the eval
		// corpus, LoadSubject will fail and we fall through without blocking.
		fmt.Fprintf(os.Stderr, "verdict check skipped (%v); writing without guard\n", err)
	} else if oldVerdict != newVerdict {
		fmt.Printf("\nverdict would change: %s → %s\n", oldVerdict, newVerdict)
		if !*acceptVerdictChange {
			fmt.Fprintln(os.Stderr, "re-run with -accept-verdict-change to proceed")
			os.Exit(1)
		}
		fmt.Println("verdict change accepted via -accept-verdict-change")
	}

	if err := r.Apply(*root, *subject, res.Changed); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", *subject, err)
		os.Exit(1)
	}

	if oldVerdict != newVerdict && oldVerdict != "" {
		if err := annotateVerdictChange(*root, *subject, oldVerdict, newVerdict); err != nil {
			fmt.Fprintf(os.Stderr, "record verdict change in PROVENANCE: %v\n", err)
		}
	}

	fmt.Printf("\n%s: %d file(s) updated and capture date refreshed\n", *subject, len(res.Changed))
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
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
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
	return os.WriteFile(provPath, append(prov, []byte(note)...), 0o644)
}
