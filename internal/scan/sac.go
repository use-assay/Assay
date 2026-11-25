package scan

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/use-assay/assay/internal/horizon"
	"github.com/use-assay/assay/internal/mechanics"
	"github.com/use-assay/assay/internal/sac"
)

// This file lets a scan start from a contract address (C…) instead of the
// classic CODE-ISSUER pair (#361).
//
// Integrators work with Stellar Asset Contract addresses — they are what the
// registry keys attestations on — and should not have to reverse-map one to
// CODE-ISSUER by hand. The contract itself is asked for its name() (which a SAC
// answers with CODE:ISSUER), and the answer is trusted only after re-deriving
// the asset's SAC address and checking it equals the input. A contract that
// returns a name for some *other* asset, or that is not a SAC at all, is
// refused: accepting its claim would attach a scanned asset's verdict to a
// different contract's address.

const (
	// DefaultTestnetRPC is the SDF-operated Stellar RPC instance for testnet.
	DefaultTestnetRPC = "https://soroban-testnet.stellar.org"
	// DefaultPubnetRPC is a public Stellar RPC instance for pubnet. The SDF
	// operates no public mainnet RPC, so this is an ecosystem provider; set
	// ASSAY_RPC_URL (or Scanner.RPCURL) to point at another one.
	DefaultPubnetRPC = "https://mainnet.sorobanrpc.com"

	// rpcMaxBody caps an RPC response read.
	rpcMaxBody = 1 << 20
)

// ErrNotSAC reports a contract address that cannot be shown to be the SAC of
// the classic asset it names.
//
// It covers every way that check fails: no contract at the address, a contract
// without a SAC name(), a name that is not CODE:ISSUER, and — the important
// one — a name whose re-derived SAC address is not the input. It is a refusal,
// not an outage: retrying will not help, and it is reported as a bad asset.
var ErrNotSAC = errors.New("scan: contract address is not a Stellar Asset Contract for the named asset")

// ErrRPC reports a transport- or protocol-level failure talking to Stellar RPC.
var ErrRPC = errors.New("scan: stellar rpc")

// errSimulation reports that the RPC answered the simulation with a contract
// error. It is private: callers see it wrapped as ErrNotSAC, because a SAC
// always exposes name() and a simulation error therefore means the address is
// not one.
var errSimulation = errors.New("simulation failed")

// IsContractAddress reports whether input names a contract (C…) rather than a
// classic asset. Contract inputs need a live RPC to resolve; classic inputs do
// not.
func IsContractAddress(input string) bool {
	return sac.IsContractAddress(strings.TrimSpace(input))
}

// DefaultRPCFor returns the public Stellar RPC endpoint for network, or "" for
// a network with no known public endpoint.
func DefaultRPCFor(network horizon.Network) string {
	switch network {
	case horizon.TestNet:
		return DefaultTestnetRPC
	case horizon.PublicNet:
		return DefaultPubnetRPC
	default:
		return ""
	}
}

// ResolveAsset parses a scan input into a classic asset.
//
// A CODE-ISSUER input is parsed locally, exactly as ParseAsset does, and needs
// no network. A C… contract address is resolved through Stellar RPC: the
// contract's name() is read, parsed as CODE:ISSUER, and accepted only if
// re-deriving the SAC address for the scanner's network gives back the input.
func (s *Scanner) ResolveAsset(ctx context.Context, input string) (mechanics.Asset, error) {
	input = strings.TrimSpace(input)
	if !IsContractAddress(input) {
		return ParseAsset(input)
	}

	network, err := s.resolveNetwork()
	if err != nil {
		return mechanics.Asset{}, err
	}
	rpcURL, err := s.rpcURL(network)
	if err != nil {
		return mechanics.Asset{}, err
	}

	// Two sequential RPC calls, each bounded by the fetch timeout, mirroring
	// the serial ledger lookups in Subject.
	resolveCtx, cancel := context.WithTimeout(ctx, 2*s.fetchTimeout())
	defer cancel()
	return ResolveContract(resolveCtx, s.rpcClient(), rpcURL, input, string(network))
}

// rpcClient is the scanner's RPC client, creating the default one when a
// Scanner was built without New (tests construct scanners by hand).
func (s *Scanner) rpcClient() *http.Client {
	if s.RPC != nil {
		return s.RPC
	}
	return &http.Client{}
}

// rpcURL is the endpoint a contract input is resolved through: the explicit
// override, then ASSAY_RPC_URL, then the public endpoint for the network.
func (s *Scanner) rpcURL(network horizon.Network) (string, error) {
	if s.RPCURL != "" {
		return s.RPCURL, nil
	}
	if env := strings.TrimSpace(os.Getenv("ASSAY_RPC_URL")); env != "" {
		return env, nil
	}
	if def := DefaultRPCFor(network); def != "" {
		return def, nil
	}
	return "", fmt.Errorf("%w: network %q has no default endpoint; set ASSAY_RPC_URL", ErrRPC, network)
}

// ResolveContract resolves a SAC address to the classic asset it represents.
//
// It refuses rather than guesses at every step:
//
//   - the RPC must serve the network the caller is scanning (a SAC address is
//     network-scoped, so a testnet answer must never be labelled pubnet);
//   - the contract must answer name() (a non-SAC contract does not);
//   - the name must parse as a classic asset; and
//   - re-deriving the SAC address from that asset must reproduce the input
//     byte for byte. A contract that names another asset fails this check,
//     which is what keeps a verdict attached to the address that was asked
//     about rather than the one a contract claims.
func ResolveContract(ctx context.Context, client *http.Client, rpcURL, contract, networkPassphrase string) (mechanics.Asset, error) {
	if !sac.IsContractAddress(contract) {
		return mechanics.Asset{}, fmt.Errorf("%w: %q is not a contract address", ErrNotSAC, contract)
	}
	if rpcURL == "" {
		return mechanics.Asset{}, fmt.Errorf("%w: no endpoint to resolve %s", ErrRPC, contract)
	}
	if networkPassphrase == "" {
		return mechanics.Asset{}, fmt.Errorf("%w: no network to resolve %s on", ErrNotSAC, contract)
	}

	served, err := rpcNetwork(ctx, client, rpcURL)
	if err != nil {
		return mechanics.Asset{}, err
	}
	if served != networkPassphrase {
		return mechanics.Asset{}, fmt.Errorf(
			"%w: rpc at %s serves %q, but the scan is of %q",
			ErrRPC, rpcURL, served, networkPassphrase)
	}

	name, err := rpcName(ctx, client, rpcURL, contract)
	if err != nil {
		if errors.Is(err, errSimulation) {
			return mechanics.Asset{}, fmt.Errorf("%w: %s does not answer name() as a SAC does: %v",
				ErrNotSAC, contract, err)
		}
		return mechanics.Asset{}, err
	}

	asset, err := ParseAsset(name)
	if err != nil {
		return mechanics.Asset{}, fmt.Errorf("%w: %s reports name %q: %v", ErrNotSAC, contract, name, err)
	}
	want, err := sac.Address(asset.Code, asset.Issuer, networkPassphrase)
	if err != nil {
		return mechanics.Asset{}, fmt.Errorf("%w: %s reports name %q: %v", ErrNotSAC, contract, name, err)
	}
	if want != contract {
		return mechanics.Asset{}, fmt.Errorf(
			"%w: %s reports name %q, but that asset's SAC address is %s",
			ErrNotSAC, contract, name, want)
	}
	return asset, nil
}

// rpcNetwork reads the passphrase of the network an RPC endpoint serves.
func rpcNetwork(ctx context.Context, client *http.Client, rpcURL string) (string, error) {
	var res struct {
		Passphrase string `json:"passphrase"`
	}
	if err := rpcCall(ctx, client, rpcURL, "getNetwork", nil, &res); err != nil {
		return "", err
	}
	if res.Passphrase == "" {
		return "", fmt.Errorf("%w: getNetwork returned no passphrase", ErrRPC)
	}
	return res.Passphrase, nil
}

// rpcName simulates name() on contract and returns the string it answered.
func rpcName(ctx context.Context, client *http.Client, rpcURL, contract string) (string, error) {
	raw, err := sac.ContractIDBytes(contract)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotSAC, err)
	}
	tx := base64.StdEncoding.EncodeToString(invokeNameEnvelope(raw))

	var res struct {
		Results []struct {
			XDR string `json:"xdr"`
		} `json:"results"`
		Error string `json:"error"`
	}
	if err := rpcCall(ctx, client, rpcURL, "simulateTransaction",
		map[string]any{"transaction": tx}, &res); err != nil {
		return "", err
	}
	if res.Error != "" {
		return "", fmt.Errorf("%w: %s", errSimulation, res.Error)
	}
	if len(res.Results) == 0 {
		return "", fmt.Errorf("%w: simulateTransaction returned no result", ErrRPC)
	}
	return decodeScString(res.Results[0].XDR)
}

// rpcCall posts one JSON-RPC request and decodes its result into out.
func rpcCall(ctx context.Context, client *http.Client, rpcURL, method string, params, out any) error {
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return fmt.Errorf("%w: encode %s request: %v", ErrRPC, method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rpcURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: build %s request: %v", ErrRPC, method, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrRPC, method, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s: status %s", ErrRPC, method, resp.Status)
	}
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, rpcMaxBody)).Decode(&env); err != nil {
		return fmt.Errorf("%w: decode %s response: %v", ErrRPC, method, err)
	}
	if env.Error != nil {
		return fmt.Errorf("%w: %s: rpc error %d: %s", ErrRPC, method, env.Error.Code, env.Error.Message)
	}
	if err := json.Unmarshal(env.Result, out); err != nil {
		return fmt.Errorf("%w: decode %s result: %v", ErrRPC, method, err)
	}
	return nil
}

// ScVal discriminants and XDR framing needed to invoke name() on a SAC. Only
// the pieces this call needs are implemented; another shape is refused rather
// than guessed at.
const (
	scvString       uint32 = 14
	xdrInvokeHostFn uint32 = 0 // HOST_FUNCTION_TYPE_INVOKE_CONTRACT
)

// invokeNameEnvelope builds the base64-ready transaction envelope for a
// simulated name() call on the contract at contractBytes.
//
//	TransactionEnvelope = [2: ENVELOPE_TYPE_TX] Transaction DecoratedSignature[]
//	Transaction         = sourceAccount fee seqNum cond memo operations ext
//	Operation           = sourceAccount* [24: INVOKE_HOST_FUNCTION] HostFunction auth<>
//	HostFunction        = [0: INVOKE_CONTRACT] ScAddress ScSymbol ScVal[]
//
// A SAC's name() is a read: it takes no arguments, and simulation does not
// check signatures or sequence numbers, so the envelope carries dummy values.
func invokeNameEnvelope(contractBytes []byte) []byte {
	var host []byte
	host = be32(host, xdrInvokeHostFn)
	host = be32(host, 1) // SC_ADDRESS_TYPE_CONTRACT
	host = append(host, contractBytes...)
	host = be32(host, uint32(len("name")))
	host = append(host, "name"...)
	for len(host)%4 != 0 {
		host = append(host, 0)
	}
	host = be32(host, 0) // args: none

	var buf []byte
	buf = be32(buf, 2)                     // ENVELOPE_TYPE_TX
	buf = be32(buf, 0)                     // sourceAccount: KEY_TYPE_ED25519
	buf = append(buf, make([]byte, 32)...) // ...and 32 zero bytes
	buf = be32(buf, 100)                   // fee
	buf = appendU64(buf, 0)                // seqNum
	buf = be32(buf, 0)                     // cond: PRECOND_NONE
	buf = be32(buf, 0)                     // memo: MEMO_NONE
	buf = be32(buf, 1)                     // operations: one
	buf = be32(buf, 0)                     // operation sourceAccount: absent
	buf = be32(buf, 24)                    // INVOKE_HOST_FUNCTION
	buf = append(buf, host...)
	buf = be32(buf, 0) // auth: none
	buf = be32(buf, 0) // transaction ext: v0
	buf = be32(buf, 0) // DecoratedSignature[]: none
	return buf
}

// decodeScString decodes a base64 ScVal holding a string.
func decodeScString(s string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", fmt.Errorf("%w: name() result is not base64: %v", ErrRPC, err)
	}
	if len(raw) < 8 {
		return "", fmt.Errorf("%w: name() result is %d bytes", ErrRPC, len(raw))
	}
	if typ := readBE32(raw, 0); typ != scvString {
		return "", fmt.Errorf("%w: name() returned ScVal type %d, want a string", ErrRPC, typ)
	}
	n := readBE32(raw, 4)
	if uint64(len(raw)) < 8+uint64(n) {
		return "", fmt.Errorf("%w: name() string truncated", ErrRPC)
	}
	return string(raw[8 : 8+n]), nil
}

func readBE32(b []byte, off int) uint32 {
	return uint32(b[off])<<24 | uint32(b[off+1])<<16 | uint32(b[off+2])<<8 | uint32(b[off+3])
}

func be32(buf []byte, v uint32) []byte {
	return append(buf, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

func appendU64(buf []byte, v uint64) []byte {
	return append(buf,
		byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32),
		byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}
