package scan_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/use-assay/assay/internal/horizon"
	"github.com/use-assay/assay/internal/sac"
	"github.com/use-assay/assay/internal/scan"
)

const sacTestIssuer = "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"

// scString encodes a Soroban ScVal string, the type a SAC's name() returns.
func scString(s string) string {
	buf := []byte{0, 0, 0, 14, byte(len(s) >> 24), byte(len(s) >> 16), byte(len(s) >> 8), byte(len(s))}
	buf = append(buf, s...)
	for len(buf)%4 != 0 {
		buf = append(buf, 0)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

// rpcStub answers getNetwork and simulateTransaction for one resolution. The
// name and err fields are whatever the test wants the contract to say.
func rpcStub(t *testing.T, network, name, simErr string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "getNetwork":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": 1,
				"result": map[string]string{"passphrase": network},
			})
		case "simulateTransaction":
			result := map[string]any{}
			if simErr != "" {
				result["error"] = simErr
			} else {
				result["results"] = []map[string]string{{"xdr": scString(name)}}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": 1, "result": result,
			})
		default:
			t.Errorf("unexpected RPC method %q", req.Method)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
}

func TestResolveContract(t *testing.T) {
	network := string(horizon.TestNet)
	contract, err := sac.Address("USDC", sacTestIssuer, network)
	if err != nil {
		t.Fatalf("derive SAC: %v", err)
	}
	other, err := sac.Address("AQUA", sacTestIssuer, network)
	if err != nil {
		t.Fatalf("derive other SAC: %v", err)
	}

	t.Run("resolves a SAC name", func(t *testing.T) {
		srv := rpcStub(t, network, "USDC:"+sacTestIssuer, "")
		defer srv.Close()

		a, err := scan.ResolveContract(context.Background(), srv.Client(), srv.URL, contract, network)
		if err != nil {
			t.Fatalf("ResolveContract: %v", err)
		}
		if a.Code != "USDC" || a.Issuer != sacTestIssuer {
			t.Errorf("ResolveContract = %+v, want USDC/%s", a, sacTestIssuer)
		}
	})

	t.Run("a name that does not round-trip is refused", func(t *testing.T) {
		// The contract at `other` claims to be USDC. Re-deriving USDC's SAC
		// gives `contract`, not `other`, so the claim must be refused.
		srv := rpcStub(t, network, "USDC:"+sacTestIssuer, "")
		defer srv.Close()

		_, err := scan.ResolveContract(context.Background(), srv.Client(), srv.URL, other, network)
		if !errors.Is(err, scan.ErrNotSAC) {
			t.Fatalf("ResolveContract = %v, want ErrNotSAC", err)
		}
	})

	t.Run("a contract without name() is refused", func(t *testing.T) {
		srv := rpcStub(t, network, "", "HostError: Error(WasmVm, MissingValue)")
		defer srv.Close()

		_, err := scan.ResolveContract(context.Background(), srv.Client(), srv.URL, contract, network)
		if !errors.Is(err, scan.ErrNotSAC) {
			t.Fatalf("ResolveContract = %v, want ErrNotSAC", err)
		}
	})

	t.Run("a name that is not CODE:ISSUER is refused", func(t *testing.T) {
		srv := rpcStub(t, network, "just a token", "")
		defer srv.Close()

		_, err := scan.ResolveContract(context.Background(), srv.Client(), srv.URL, contract, network)
		if !errors.Is(err, scan.ErrNotSAC) {
			t.Fatalf("ResolveContract = %v, want ErrNotSAC", err)
		}
	})

	t.Run("an RPC on another network is refused", func(t *testing.T) {
		srv := rpcStub(t, string(horizon.PublicNet), "USDC:"+sacTestIssuer, "")
		defer srv.Close()

		_, err := scan.ResolveContract(context.Background(), srv.Client(), srv.URL, contract, network)
		if !errors.Is(err, scan.ErrRPC) {
			t.Fatalf("ResolveContract = %v, want ErrRPC", err)
		}
	})

	t.Run("a non-contract address is refused", func(t *testing.T) {
		_, err := scan.ResolveContract(context.Background(), http.DefaultClient, "https://rpc.invalid", "USDC-"+sacTestIssuer, network)
		if !errors.Is(err, scan.ErrNotSAC) {
			t.Fatalf("ResolveContract = %v, want ErrNotSAC", err)
		}
	})
}

// failingRPC fails the test if anything asks it to resolve, so the classic
// path is shown to take no network at all.
func failingRPC(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("classic input made an RPC request: %s", r.URL)
	}))
}

func TestResolveAssetClassicNeedsNoRPC(t *testing.T) {
	srv := failingRPC(t)
	defer srv.Close()

	s := &scan.Scanner{
		Horizon: horizon.New("https://horizon.example.invalid"),
		Network: horizon.TestNet,
		RPC:     srv.Client(),
		RPCURL:  srv.URL,
	}
	a, err := s.ResolveAsset(context.Background(), "USDC:"+sacTestIssuer)
	if err != nil {
		t.Fatalf("ResolveAsset: %v", err)
	}
	if a.Code != "USDC" || a.Issuer != sacTestIssuer {
		t.Errorf("ResolveAsset = %+v", a)
	}
}

func TestResolveAssetContractUsesRPC(t *testing.T) {
	network := string(horizon.TestNet)
	contract, err := sac.Address("USDC", sacTestIssuer, network)
	if err != nil {
		t.Fatalf("derive SAC: %v", err)
	}
	srv := rpcStub(t, network, "USDC:"+sacTestIssuer, "")
	defer srv.Close()

	s := &scan.Scanner{
		Horizon: horizon.New("https://horizon.example.invalid"),
		Network: horizon.TestNet,
		RPC:     srv.Client(),
		RPCURL:  srv.URL,
	}
	a, err := s.ResolveAsset(context.Background(), contract)
	if err != nil {
		t.Fatalf("ResolveAsset: %v", err)
	}
	if a.Code != "USDC" || a.Issuer != sacTestIssuer {
		t.Errorf("ResolveAsset = %+v, want USDC/%s", a, sacTestIssuer)
	}
}
