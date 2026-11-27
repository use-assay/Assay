// Package sacadmin reads the administrator of a Stellar Asset Contract from a
// Soroban RPC ledger entry.
//
// A classic asset's Stellar Asset Contract has an admin that defaults to the
// issuer when the SAC is first enabled, and can be moved with the SAC's
// set_admin function. The admin defaults to the issuer, and changing it requires
// authorization from the current admin — the issuer, the first time. A SAC admin
// may be a Stellar account (G…) or another smart contract (C…)
// (CAP-46-06, "Stellar Asset Contract"; developers.stellar.org, "Set a custom
// admin account for a Stellar Asset Contract").
//
// This matters because the issuer flags Assay already reads describe the issuer
// account, not the SAC admin. Once the admin has moved, the two are independent:
// the issuer can be locked while the SAC admin still mints, claws back or
// freezes through the contract. Clawback through the SAC still requires the
// classic asset to have AUTH_CLAWBACK_ENABLED set, and set_authorized still
// requires AUTH_REVOCABLE_FLAG (CAP-46-06), but those flags no longer describe
// who can exercise the power.
//
// The admin is stored in the SAC's contract-instance storage under the key
// `Admin`. There is no Horizon endpoint for it, so this package builds the
// LedgerKey for the contract instance by hand, asks Soroban RPC
// getLedgerEntries, and decodes the returned LedgerEntryData XDR. It reads only
// the fields it needs and refuses any payload it cannot read exactly.
package sacadmin

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/use-assay/assay/internal/horizon"
)

// DefaultTestnetURL is the SDF-operated Soroban RPC instance for testnet. There
// is no endpoint for pubnet that this repository is willing to treat as
// authoritative, so a pubnet scan must name one explicitly; see
// Scanner.SACAdminRPC.
const DefaultTestnetURL = "https://soroban-testnet.stellar.org"

// MaxBody caps an RPC response read, on the same principle as the other
// fetchers: a broken or hostile server must not be able to stream an unbounded
// body at the scanner. One contract-instance entry is a few hundred bytes of
// base64.
const MaxBody = 1 << 20

// Sentinel errors. Callers must treat every error as "the admin was not read",
// never as "the admin is the issuer".
var (
	// ErrBadContractID reports a contract address that is not a valid C… strkey,
	// so no ledger key can name it.
	ErrBadContractID = errors.New("sacadmin: invalid contract address")
	// ErrEntryAbsent reports that RPC answered with no ledger entry for the
	// contract instance. The contract is not deployed, or the entry was pruned.
	ErrEntryAbsent = errors.New("sacadmin: no contract instance entry")
	// ErrNotContractData reports a ledger entry whose discriminant is not
	// CONTRACT_DATA.
	ErrNotContractData = errors.New("sacadmin: ledger entry is not contract data")
	// ErrTruncated reports XDR too short for the fields being read.
	ErrTruncated = errors.New("sacadmin: contract entry XDR is truncated")
	// ErrNoAdmin reports a readable contract instance with no Admin storage
	// entry. It is a fact about the contract, not a transport failure.
	ErrNoAdmin = errors.New("sacadmin: contract instance has no Admin entry")
	// ErrUnknownXDR reports an XDR union tag or shape this parser does not know.
	// It refuses rather than guessing, because a shifted read would attribute a
	// different address's admin to this contract.
	ErrUnknownXDR = errors.New("sacadmin: unrecognized XDR")
	// ErrRPC reports a transport- or protocol-level failure talking to RPC.
	ErrRPC = errors.New("sacadmin: soroban rpc")
)

// Client reads contract-instance entries from a Soroban RPC endpoint.
type Client struct {
	BaseURL   string
	HTTP      *http.Client
	UserAgent string
}

// New returns a Client for the given Soroban RPC URL, defaulting to the SDF
// testnet instance.
func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultTestnetURL
	}
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 15 * time.Second},
		UserAgent: "assay/" + horizon.Version +
			" (+https://github.com/use-assay/Assay)",
	}
}

// Admin returns the admin address of the Stellar Asset Contract at sacAddress,
// as a G… or C… strkey. Any error means the admin was not read.
func (c *Client) Admin(ctx context.Context, sacAddress string) (string, error) {
	key, err := LedgerKeyForSAC(sacAddress)
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getLedgerEntries",
		"params":  map[string]any{"keys": []string{key}},
	})
	if err != nil {
		return "", fmt.Errorf("%w: encode request: %w", ErrRPC, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, strings.NewReader(string(payload)))
	if err != nil {
		return "", fmt.Errorf("%w: build request: %w", ErrRPC, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: post %s: %w", ErrRPC, c.BaseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: status %d", ErrRPC, resp.StatusCode)
	}

	var body struct {
		Result *struct {
			Entries []struct {
				Key string `json:"key"`
				XDR string `json:"xdr"`
			} `json:"entries"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, MaxBody))
	if err := dec.Decode(&body); err != nil {
		return "", fmt.Errorf("%w: decode response: %w", ErrRPC, err)
	}
	if body.Error != nil {
		return "", fmt.Errorf("%w: rpc error %d: %s", ErrRPC, body.Error.Code, body.Error.Message)
	}
	if body.Result == nil || len(body.Result.Entries) == 0 {
		return "", fmt.Errorf("%w: %s", ErrEntryAbsent, sacAddress)
	}
	for _, e := range body.Result.Entries {
		if e.Key != key {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(e.XDR)
		if err != nil {
			return "", fmt.Errorf("%w: decode entry xdr: %w", ErrRPC, err)
		}
		return ParseInstanceAdmin(raw)
	}
	return "", fmt.Errorf("%w: response did not echo the requested key", ErrRPC)
}

// LedgerKeyForSAC builds the base64 LedgerKey addressing the contract instance
// of the SAC at sacAddress.
//
// The encoding is the protocol's, not a convention:
//
//	LedgerKey = [6: CONTRACT_DATA] LedgerKeyContractData
//	LedgerKeyContractData = {
//	    contract:   SCAddress,   // [1: SC_ADDRESS_TYPE_CONTRACT] Hash(32)
//	    key:        SCVal,       // [20: SCV_LEDGER_KEY_CONTRACT_INSTANCE]
//	    durability: PERSISTENT,  // 1; instance storage is persistent
//	}
func LedgerKeyForSAC(sacAddress string) (string, error) {
	id, err := decodeContractStrKey(sacAddress)
	if err != nil {
		return "", err
	}
	buf := make([]byte, 0, 4+4+32+4+4)
	buf = binary.BigEndian.AppendUint32(buf, ledgerEntryTypeContractData)
	buf = binary.BigEndian.AppendUint32(buf, scAddressTypeContract)
	buf = append(buf, id...)
	buf = binary.BigEndian.AppendUint32(buf, scvLedgerKeyContractInstance)
	buf = binary.BigEndian.AppendUint32(buf, contractDataDurable)
	return base64.StdEncoding.EncodeToString(buf), nil
}

// XDR tags used by this package. They are fixed by the Stellar protocol
// (Stellar-ledger-entries.x, Stellar-contract.x, Stellar-contract-env.x).
const (
	ledgerEntryTypeContractData = 6

	scAddressTypeAccount  = 0
	scAddressTypeContract = 1

	contractDataTemporary = 0
	contractDataDurable   = 1

	scvBool                        = 0
	scvVoid                        = 1
	scvError                       = 2
	scvU32                         = 3
	scvI32                         = 4
	scvU64                         = 5
	scvI64                         = 6
	scvTimepoint                   = 7
	scvDuration                    = 8
	scvU128                        = 9
	scvI128                        = 10
	scvU256                        = 11
	scvI256                        = 12
	scvBytes                       = 13
	scvString                      = 14
	scvSymbol                      = 15
	scvVec                         = 16
	scvMap                         = 17
	scvAddress                     = 18
	scvContractInstance            = 19
	scvLedgerKeyContractInstance   = 20
	scvLedgerKeyNonce              = 21
	contractExecutableWasm         = 0
	contractExecutableStellarAsset = 1
)

// ParseInstanceAdmin extracts the Admin address from a raw CONTRACT_DATA
// LedgerEntryData payload, exactly as Soroban RPC returns it (the `xdr` field,
// base64, already decoded by the caller).
//
// The layout it reads, from the start of LedgerEntryData:
//
//	0:4    LedgerEntryData discriminant; 6 = CONTRACT_DATA
//	4:8    ContractDataEntry.ext; 0
//	8:12   SCAddress discriminant; 1 = contract
//	12:44  the 32-byte contract id
//	44:48  SCVal discriminant; 20 = SCV_LEDGER_KEY_CONTRACT_INSTANCE
//	48:52  durability
//	52:…   SCVal: ContractInstance
//
// The instance holds a nullable storage map; the Admin entry's key is a
// single-element vector containing the symbol "Admin", and its value is an
// SCAddress. Addresses are returned as strkeys so the caller can compare them
// to the issuer account directly.
func ParseInstanceAdmin(entryXDR []byte) (string, error) {
	r := reader{b: entryXDR}
	t, err := r.u32()
	if err != nil {
		return "", err
	}
	if t != ledgerEntryTypeContractData {
		return "", fmt.Errorf("%w: discriminant %d", ErrNotContractData, t)
	}
	if _, err := r.u32(); err != nil { // ext must be 0; value irrelevant to the read
		return "", err
	}
	addrType, err := r.u32()
	if err != nil {
		return "", err
	}
	if addrType != scAddressTypeContract {
		return "", fmt.Errorf("%w: contract address type %d", ErrUnknownXDR, addrType)
	}
	if _, err := r.bytes(32); err != nil {
		return "", err
	}
	keyType, err := r.u32()
	if err != nil {
		return "", err
	}
	if keyType != scvLedgerKeyContractInstance {
		return "", fmt.Errorf("%w: instance key SCVal type %d", ErrUnknownXDR, keyType)
	}
	if _, err := r.u32(); err != nil { // durability
		return "", err
	}

	valType, err := r.u32()
	if err != nil {
		return "", err
	}
	if valType != scvContractInstance {
		return "", fmt.Errorf("%w: instance value SCVal type %d", ErrUnknownXDR, valType)
	}
	execType, err := r.u32()
	if err != nil {
		return "", err
	}
	switch execType {
	case contractExecutableWasm:
		if _, err := r.bytes(32); err != nil {
			return "", err
		}
	case contractExecutableStellarAsset:
		// No payload.
	default:
		return "", fmt.Errorf("%w: executable type %d", ErrUnknownXDR, execType)
	}

	present, err := r.u32()
	if err != nil {
		return "", err
	}
	if present == 0 {
		return "", ErrNoAdmin
	}
	count, err := r.u32()
	if err != nil {
		return "", err
	}
	// The map is small (a handful of entries). A count that cannot be plausibly
	// backed by the remaining bytes is a corruption, not a request to loop.
	if count > uint32(len(entryXDR)) {
		return "", fmt.Errorf("%w: storage map count %d", ErrTruncated, count)
	}
	for i := uint32(0); i < count; i++ {
		isAdmin, err := readStorageKey(&r)
		if err != nil {
			return "", err
		}
		if isAdmin {
			return readAddress(&r)
		}
		if err := skipSCVal(&r); err != nil {
			return "", err
		}
	}
	return "", ErrNoAdmin
}

// readStorageKey consumes an SCVal storage key and reports whether it is the
// single-element vector [Symbol("Admin")] the SAC uses for its administrator.
//
// Every key this function does not recognize is still fully consumed, so the
// stream stays aligned for the value that follows; only the Admin shape is
// interpreted.
func readStorageKey(r *reader) (bool, error) {
	t, err := r.u32()
	if err != nil {
		return false, err
	}
	if t != scvVec {
		if err := skipSCValBody(r, t); err != nil {
			return false, err
		}
		return false, nil
	}
	// SCVec is a nullable pointer in SCVal: the presence flag comes before the
	// element count.
	present, err := r.u32()
	if err != nil {
		return false, err
	}
	if present == 0 {
		return false, nil
	}
	n, err := r.u32()
	if err != nil {
		return false, err
	}
	if n != 1 {
		for i := uint32(0); i < n; i++ {
			if err := skipSCVal(r); err != nil {
				return false, err
			}
		}
		return false, nil
	}
	et, err := r.u32()
	if err != nil {
		return false, err
	}
	if et != scvSymbol {
		if err := skipSCValBody(r, et); err != nil {
			return false, err
		}
		return false, nil
	}
	sym, err := r.varOpaque()
	if err != nil {
		return false, err
	}
	return string(sym) == "Admin", nil
}

// readAddress consumes an SCVal whose type must be ADDRESS, and renders the
// address as a G… or C… strkey.
func readAddress(r *reader) (string, error) {
	t, err := r.u32()
	if err != nil {
		return "", err
	}
	if t != scvAddress {
		return "", fmt.Errorf("%w: Admin value SCVal type %d", ErrUnknownXDR, t)
	}
	kind, err := r.u32()
	if err != nil {
		return "", err
	}
	switch kind {
	case scAddressTypeAccount:
		// SCAddress's account case holds an AccountID, which is itself a
		// PublicKey union: [0: ED25519] Uint256.
		pkType, err := r.u32()
		if err != nil {
			return "", err
		}
		if pkType != 0 {
			return "", fmt.Errorf("%w: account public key type %d", ErrUnknownXDR, pkType)
		}
		raw, err := r.bytes(32)
		if err != nil {
			return "", err
		}
		return encodeStrKey(strKeyVersionAccountID, raw), nil
	case scAddressTypeContract:
		raw, err := r.bytes(32)
		if err != nil {
			return "", err
		}
		return encodeStrKey(strKeyVersionContract, raw), nil
	default:
		return "", fmt.Errorf("%w: SCAddress type %d", ErrUnknownXDR, kind)
	}
}

// skipSCVal consumes one complete SCVal, reading its type tag and then the
// type-specific body.
func skipSCVal(r *reader) error {
	t, err := r.u32()
	if err != nil {
		return err
	}
	return skipSCValBody(r, t)
}

// skipSCValBody consumes the body of an SCVal whose type tag has already been
// read.
func skipSCValBody(r *reader, t uint32) error {
	switch t {
	case scvBool:
		_, err := r.u32()
		return err
	case scvVoid:
		return nil
	case scvError:
		_, err := r.u32()
		return err
	case scvU32, scvI32:
		_, err := r.bytes(4)
		return err
	case scvU64, scvI64, scvTimepoint, scvDuration:
		_, err := r.bytes(8)
		return err
	case scvU128, scvI128:
		_, err := r.bytes(16)
		return err
	case scvU256, scvI256:
		_, err := r.bytes(32)
		return err
	case scvBytes, scvString, scvSymbol:
		_, err := r.varOpaque()
		return err
	case scvVec:
		present, err := r.u32()
		if err != nil {
			return err
		}
		if present == 0 {
			return nil
		}
		n, err := r.u32()
		if err != nil {
			return err
		}
		if n > uint32(len(r.b)) {
			return fmt.Errorf("%w: vec count %d", ErrTruncated, n)
		}
		for i := uint32(0); i < n; i++ {
			if err := skipSCVal(r); err != nil {
				return err
			}
		}
		return nil
	case scvMap:
		present, err := r.u32()
		if err != nil {
			return err
		}
		if present == 0 {
			return nil
		}
		n, err := r.u32()
		if err != nil {
			return err
		}
		if n > uint32(len(r.b)) {
			return fmt.Errorf("%w: map count %d", ErrTruncated, n)
		}
		for i := uint32(0); i < n; i++ {
			if err := skipSCVal(r); err != nil {
				return err
			}
			if err := skipSCVal(r); err != nil {
				return err
			}
		}
		return nil
	case scvAddress:
		kind, err := r.u32()
		if err != nil {
			return err
		}
		switch kind {
		case scAddressTypeAccount:
			pkType, err := r.u32()
			if err != nil {
				return err
			}
			if pkType != 0 {
				return fmt.Errorf("%w: account public key type %d", ErrUnknownXDR, pkType)
			}
			_, err = r.bytes(32)
			return err
		case scAddressTypeContract:
			_, err = r.bytes(32)
			return err
		default:
			return fmt.Errorf("%w: SCAddress type %d", ErrUnknownXDR, kind)
		}
	case scvLedgerKeyNonce:
		_, err := r.bytes(32)
		return err
	default:
		return fmt.Errorf("%w: SCVal type %d", ErrUnknownXDR, t)
	}
}

// reader is a bounds-checked big-endian XDR cursor. Every read refuses rather
// than reading past the end of the payload.
type reader struct {
	b   []byte
	off int
}

func (r *reader) bytes(n int) ([]byte, error) {
	if n < 0 || r.off+n > len(r.b) {
		return nil, fmt.Errorf("%w: need %d bytes at offset %d, have %d",
			ErrTruncated, n, r.off, len(r.b)-r.off)
	}
	out := r.b[r.off : r.off+n]
	r.off += n
	return out, nil
}

func (r *reader) u32() (uint32, error) {
	b, err := r.bytes(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b), nil
}

// varOpaque reads XDR variable-length opaque data: a uint32 length followed by
// that many bytes, padded to a four-byte boundary.
func (r *reader) varOpaque() ([]byte, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	if n > uint32(len(r.b)) {
		return nil, fmt.Errorf("%w: opaque length %d", ErrTruncated, n)
	}
	out, err := r.bytes(int(n))
	if err != nil {
		return nil, err
	}
	pad := (4 - int(n)%4) % 4
	if _, err := r.bytes(pad); err != nil {
		return nil, err
	}
	return out, nil
}

// Strkey version bytes (SEP-23): the high five bits of the first decoded byte.
const (
	strKeyVersionAccountID byte = 6 << 3 // 'G'
	strKeyVersionContract  byte = 2 << 3 // 'C'
	strKeyAlphabet              = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
)

// decodeContractStrKey decodes a C… address to its 32-byte contract id,
// verifying the length, alphabet and CRC-16 checksum. A malformed contract
// address must never become a ledger key naming some other contract.
func decodeContractStrKey(s string) ([]byte, error) {
	if len(s) != 56 {
		return nil, fmt.Errorf("%w: %q is %d characters, want 56", ErrBadContractID, s, len(s))
	}
	raw := make([]byte, 0, 35)
	var acc uint32
	var bits uint8
	for i := 0; i < len(s); i++ {
		v := strings.IndexByte(strKeyAlphabet, s[i])
		if v < 0 {
			return nil, fmt.Errorf("%w: %q has a character outside the base32 alphabet", ErrBadContractID, s)
		}
		acc = acc<<5 | uint32(v)
		bits += 5
		if bits >= 8 {
			bits -= 8
			raw = append(raw, byte(acc>>bits))
		}
	}
	if len(raw) != 35 {
		return nil, fmt.Errorf("%w: %q decoded to %d bytes, want 35", ErrBadContractID, s, len(raw))
	}
	if raw[0] != strKeyVersionContract {
		return nil, fmt.Errorf("%w: %q is not a contract strkey", ErrBadContractID, s)
	}
	if want := strKeyChecksum(raw[:33]); want[0] != raw[33] || want[1] != raw[34] {
		return nil, fmt.Errorf("%w: %q fails its checksum", ErrBadContractID, s)
	}
	return raw[1:33], nil
}

// encodeStrKey renders a version byte and 32-byte payload as a Stellar strkey.
func encodeStrKey(version byte, payload []byte) string {
	raw := make([]byte, 0, 35)
	raw = append(raw, version)
	raw = append(raw, payload...)
	c := strKeyChecksum(raw)
	raw = append(raw, c[0], c[1])

	var b strings.Builder
	b.Grow(56)
	var acc uint32
	var bits uint8
	for _, by := range raw {
		acc = acc<<8 | uint32(by)
		bits += 8
		for bits >= 5 {
			bits -= 5
			b.WriteByte(strKeyAlphabet[(acc>>bits)&0x1f])
		}
	}
	if bits > 0 {
		b.WriteByte(strKeyAlphabet[(acc<<(5-bits))&0x1f])
	}
	return b.String()
}

// strKeyChecksum computes the CRC-16 checksum Stellar appends to strkeys: the
// non-reflected CCITT polynomial 0x1021, initial value 0, no final complement,
// appended little-endian.
func strKeyChecksum(data []byte) [2]byte {
	crc := uint16(0)
	for _, b := range data {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return [2]byte{byte(crc & 0xff), byte(crc >> 8)}
}
