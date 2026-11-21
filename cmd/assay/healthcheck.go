package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// defaultHealthcheckURL matches the address `assay serve` listens on by
// default. It is what the container image's HEALTHCHECK invokes: the image is
// distroless, so there is no shell or curl to probe with, and the binary can
// answer the same question itself.
const defaultHealthcheckURL = "http://127.0.0.1:8080/healthz"

// runHealthcheck implements `assay healthcheck`. It exits successfully only
// when the endpoint answers 200 with the exact liveness body. Any other answer
// — a connection failure, a non-200 status, or an unexpected body — is an
// error, because a healthcheck that accepts "something replied" would report a
// broken process as healthy.
func runHealthcheck(args []string) error {
	fs := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	url := fs.String("url", defaultHealthcheckURL, "URL of the assay /healthz endpoint")
	timeout := fs.Duration("timeout", 3*time.Second, "overall request timeout")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return usagef("healthcheck takes no arguments (got %q)", fs.Arg(0))
	}

	client := &http.Client{Timeout: *timeout}
	resp, err := client.Get(*url)
	if err != nil {
		return fmt.Errorf("healthcheck: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck: %s returned %s", *url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return fmt.Errorf("healthcheck: read %s: %w", *url, err)
	}
	if got := strings.TrimSpace(string(body)); got != "ok" {
		return fmt.Errorf("healthcheck: %s body = %q, want %q", *url, got, "ok")
	}
	return nil
}
