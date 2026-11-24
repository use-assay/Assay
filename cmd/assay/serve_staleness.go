package main

import (
	"fmt"
	"time"

	"github.com/use-assay/assay/internal/api"
	"github.com/use-assay/assay/internal/attester"
	"github.com/use-assay/assay/internal/registry"
	"github.com/use-assay/assay/internal/sac"
)

// newStalenessReader builds the registry client /readyz reads attested_at
// through. It is a variable so a test can substitute a stub reader and never
// reach a Soroban RPC.
var newStalenessReader = func(rpcURL, contractID string) api.RegistrySafetyReader {
	return registry.New(rpcURL, contractID)
}

// registryStalenessProber builds the /readyz registry-staleness prober from an
// attested-set file (the attested.json format, read by internal/attester).
//
// An empty path returns nil, which leaves /readyz exactly as it was: staleness
// reported as unknown because it is not configured. A file that cannot be read,
// does not validate, or names an asset whose SAC cannot be derived returns a
// prober carrying only ConfigurationError, so the server still starts and
// /readyz reports the parse error as the reason for "unknown" — the registry
// block is informational, and a bad config for it must not take scans down.
//
// window is how old an attestation may be before it counts as stale. Zero
// means the file's cadence: an attestation older than one cadence is one whose
// scheduled re-attestation did not land.
func registryStalenessProber(path, rpcURL string, window time.Duration) *api.RegistryStalenessProber {
	if path == "" {
		return nil
	}
	bad := func(format string, args ...any) *api.RegistryStalenessProber {
		return &api.RegistryStalenessProber{
			ConfigurationError: "registry staleness: " + fmt.Sprintf(format, args...),
		}
	}

	cfg, err := attester.LoadConfig(path)
	if err != nil {
		return bad("%v", err)
	}
	if window < 0 {
		return bad("-staleness-window must not be negative, got %s", window)
	}
	if window == 0 {
		if cfg.Cadence == "" {
			return bad("%s sets no cadence and -staleness-window was not given, so there is no window to measure staleness against", path)
		}
		// The config has been validated, so the cadence parses.
		window, _ = time.ParseDuration(cfg.Cadence)
		if window <= 0 {
			return bad("%s cadence %q is not a positive duration; pass -staleness-window", path, cfg.Cadence)
		}
	}

	// The SAC is derived for the registry's network, not the asset's: the
	// registry is keyed by the contract address on the network it lives on.
	passphrase := cfg.RegistryPassphrase()
	assets := make([]api.RegistryAsset, 0, len(cfg.Assets))
	for _, a := range cfg.ParsedAssets() {
		addr, err := sac.Address(a.Code, a.Issuer, passphrase)
		if err != nil {
			return bad("derive SAC for %s: %v", a, err)
		}
		assets = append(assets, api.RegistryAsset{Asset: a.String(), SAC: addr})
	}

	return &api.RegistryStalenessProber{
		Reader: newStalenessReader(rpcURL, cfg.ContractID),
		Assets: assets,
		Window: window,
	}
}
