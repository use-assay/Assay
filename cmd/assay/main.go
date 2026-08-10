// Command assay scans Stellar assets for issuer trap mechanics.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/use-assay/assay/internal/api"
	"github.com/use-assay/assay/internal/scan"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "assay:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage:
  assay scan CODE-ISSUER   classify one asset and print the report as JSON
  assay serve [-addr]      serve the HTTP API and UI
`)
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return fmt.Errorf("no command given")
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	switch args[0] {
	case "scan":
		return runScan(args[1:])
	case "serve":
		return runServe(args[1:], log)
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runScan(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("scan takes exactly one asset (CODE-ISSUER)")
	}
	asset, err := scan.ParseAsset(args[0])
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	report, err := scan.New().Scan(ctx, asset)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func runServe(args []string, log *slog.Logger) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           api.NewServer(log).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Info("assay listening", "addr", *addr)
	return srv.ListenAndServe()
}
