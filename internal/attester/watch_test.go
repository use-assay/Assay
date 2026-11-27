package attester

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/use-assay/assay/internal/horizon"
)

const (
	watchedIssuer = "GA22QHSHQEHDJS2ZOINSC77XPPQ24G5EFRJGVEIZLKC5FAW3PQ5XNSDQ"
	otherAccount  = "GB7TAYRUZGE6TVT7NHP5SMIZRNQA6PLM423EYISAOAP3MKYIQMVYP2JO"
)

func TestApplyOperation(t *testing.T) {
	start := horizon.Flags{AuthRequired: true}
	for name, tc := range map[string]struct {
		op          horizon.Operation
		want        horizon.Flags
		wantChanged bool
	}{
		"not set_options": {
			horizon.Operation{Type: "payment", TransactionSuccessful: true, SourceAccount: watchedIssuer},
			start, false,
		},
		"failed transaction": {
			horizon.Operation{Type: "set_options", SetFlags: []int{8}, SourceAccount: watchedIssuer},
			start, false,
		},
		"another account's set_options": {
			horizon.Operation{Type: "set_options", TransactionSuccessful: true, SetFlags: []int{8}, SourceAccount: otherAccount},
			start, false,
		},
		"no-op: sets a flag already set": {
			horizon.Operation{Type: "set_options", TransactionSuccessful: true, SetFlags: []int{1}, SourceAccount: watchedIssuer},
			start, false,
		},
		"no-op: clears a flag already clear": {
			horizon.Operation{Type: "set_options", TransactionSuccessful: true, ClearFlags: []int{8}, SourceAccount: watchedIssuer},
			start, false,
		},
		"sets clawback": {
			horizon.Operation{Type: "set_options", TransactionSuccessful: true, SetFlags: []int{8}, SourceAccount: watchedIssuer},
			horizon.Flags{AuthRequired: true, AuthClawbackEnabled: true}, true,
		},
		"clears required, sets revocable and immutable": {
			horizon.Operation{Type: "set_options", TransactionSuccessful: true, ClearFlags: []int{1}, SetFlags: []int{2, 4}, SourceAccount: watchedIssuer},
			horizon.Flags{AuthRevocable: true, AuthImmutable: true}, true,
		},
	} {
		got, changed := ApplyOperation(start, watchedIssuer, tc.op)
		if got != tc.want || changed != tc.wantChanged {
			t.Errorf("%s: got %+v changed=%v, want %+v changed=%v", name, got, changed, tc.want, tc.wantChanged)
		}
	}
}

// fakeHorizon serves a fixture operation stream for one issuer.
type fakeHorizon struct {
	mu       sync.Mutex
	pages    map[string][]byte // cursor -> page served for an ascending request
	requests []string
}

func newFixtureHorizon(t *testing.T) (*httptest.Server, *fakeHorizon) {
	t.Helper()
	page, err := os.ReadFile("testdata/operations-after-cursor.json")
	if err != nil {
		t.Fatal(err)
	}
	empty := []byte(`{"_embedded":{"records":[]}}`)
	f := &fakeHorizon{pages: map[string][]byte{
		"250000000000000000": page,
		"250000000000000005": empty,
	}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.URL.RequestURI())
		f.mu.Unlock()
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/accounts/"+watchedIssuer:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"account_id": watchedIssuer,
				// Horizon always publishes all four flags, and the decoder
				// refuses a partial set (#530), so the fixture does too.
				"flags": map[string]bool{
					"auth_required":         true,
					"auth_revocable":        false,
					"auth_immutable":        false,
					"auth_clawback_enabled": false,
				},
			})
		case r.URL.Path == "/accounts/"+watchedIssuer+"/operations" && q.Get("order") == "desc":
			_, _ = w.Write([]byte(`{"_embedded":{"records":[{"id":"250000000000000000","paging_token":"250000000000000000","type":"payment","transaction_successful":true}]}}`))
		case r.URL.Path == "/accounts/"+watchedIssuer+"/operations":
			body, ok := f.pages[q.Get("cursor")]
			if !ok {
				http.Error(w, "unexpected cursor "+q.Get("cursor"), http.StatusBadRequest)
				return
			}
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, f
}

func watchConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := ParseConfig([]byte(`{
		"network": "testnet",
		"contract_id": "CBK4FBIHMDTXCUPE4E3ZDVSFJSCY5FJETTKNIQPN4LFJIKKIBLKIXQ73",
		"assets": [
			{"asset": "BERKSHIRE-GA22QHSHQEHDJS2ZOINSC77XPPQ24G5EFRJGVEIZLKC5FAW3PQ5XNSDQ"},
			{"asset": "BRK-GA22QHSHQEHDJS2ZOINSC77XPPQ24G5EFRJGVEIZLKC5FAW3PQ5XNSDQ"}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestWatcherDetectsFlagChangeOnFixtureStream(t *testing.T) {
	srv, fake := newFixtureHorizon(t)
	w := NewWatcher(watchConfig(t), horizon.New(srv.URL))
	ctx := context.Background()
	if err := w.Init(ctx); err != nil {
		t.Fatal(err)
	}

	got, err := w.Poll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []FlagTransition{{
		Issuer: watchedIssuer,
		Assets: []string{
			"BERKSHIRE-" + watchedIssuer,
			"BRK-" + watchedIssuer,
		},
		From:        horizon.Flags{AuthRequired: true},
		To:          horizon.Flags{AuthRequired: true, AuthRevocable: true, AuthClawbackEnabled: true},
		Added:       []string{"auth_clawback_enabled", "auth_revocable"},
		OperationID: "250000000000000005",
		TxHash:      "aa00000000000000000000000000000000000000000000000000000000000005",
		At:          "2026-10-20T09:20:00Z",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("transitions:\n got %+v\nwant %+v", got, want)
	}

	// The cursor advanced past the whole page: the next poll asks from the
	// last operation and, with nothing new, reports nothing.
	again, err := w.Poll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("second poll reported %+v, want nothing", again)
	}

	// One issuer backs both assets, so it is polled once per pass, and no
	// request ever relies on Horizon's unreliable server-side type filter.
	for _, req := range fake.requests {
		if strings.Contains(req, "type=") {
			t.Errorf("request %s uses a server-side type filter", req)
		}
	}
	var opsRequests int
	for _, req := range fake.requests {
		if strings.Contains(req, "/operations?") && strings.Contains(req, "order=asc") {
			opsRequests++
		}
	}
	if opsRequests != 2 {
		t.Errorf("ascending operations requests = %d, want 2 (one per poll)", opsRequests)
	}
}

func TestWatcherKeepsCursorWhenPollFails(t *testing.T) {
	srv, fake := newFixtureHorizon(t)
	w := NewWatcher(watchConfig(t), horizon.New(srv.URL))
	ctx := context.Background()
	if err := w.Init(ctx); err != nil {
		t.Fatal(err)
	}

	// Horizon fails for the first poll: nothing is reported and nothing is
	// skipped, because the cursor does not move.
	fake.mu.Lock()
	page := fake.pages["250000000000000000"]
	delete(fake.pages, "250000000000000000")
	fake.mu.Unlock()
	if _, err := w.Poll(ctx); err == nil {
		t.Fatal("poll against a failing Horizon returned no error")
	}

	fake.mu.Lock()
	fake.pages["250000000000000000"] = page
	fake.mu.Unlock()
	got, err := w.Poll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].OperationID != "250000000000000005" {
		t.Fatalf("after recovery got %+v, want the one flag change", got)
	}
}

// memHorizon is an in-process HorizonReader: one issuer's operation log and
// current flags, which a test appends to while the watcher is "down".
type memHorizon struct {
	flags horizon.Flags
	ops   []horizon.Operation
}

func (m *memHorizon) Account(_ context.Context, id string) (*horizon.Account, error) {
	return &horizon.Account{AccountID: id, Flags: m.flags}, nil
}

func (m *memHorizon) Operations(_ context.Context, _ string, cursor string, order horizon.Order, limit int) ([]horizon.Operation, error) {
	if order == horizon.Descending {
		if len(m.ops) == 0 {
			return nil, nil
		}
		return []horizon.Operation{m.ops[len(m.ops)-1]}, nil
	}
	var out []horizon.Operation
	for _, op := range m.ops {
		if cursor == "" || op.PagingToken > cursor {
			out = append(out, op)
		}
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

// setOptions appends a successful set_options by the issuer and applies it to
// the current flags, as the ledger would.
func (m *memHorizon) setOptions(token string, set, clear []int) {
	op := horizon.Operation{
		ID: token, PagingToken: token, Type: "set_options", TransactionSuccessful: true,
		SourceAccount: watchedIssuer, SetFlags: set, ClearFlags: clear,
	}
	m.flags, _ = ApplyOperation(m.flags, watchedIssuer, op)
	m.ops = append(m.ops, op)
}

func TestWatcherStateReplaysOperationsMissedDuringDowntime(t *testing.T) {
	ctx := context.Background()
	statePath := filepath.Join(t.TempDir(), "watch-state.json")
	h := &memHorizon{flags: horizon.Flags{AuthRequired: true}}
	h.ops = append(h.ops, horizon.Operation{ID: "100", PagingToken: "100", Type: "payment", TransactionSuccessful: true})

	first := NewWatcher(watchConfig(t), h)
	first.SetStatePath(statePath)
	if err := first.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("Init did not write the baseline: %v", err)
	}

	// The watcher is down while the issuer sets and then clears clawback:
	// the current flags end where they started, so only the replay sees it.
	h.setOptions("101", []int{flagAuthClawbackEnabled}, nil)
	h.setOptions("102", nil, []int{flagAuthClawbackEnabled})

	restarted := NewWatcher(watchConfig(t), h)
	restarted.SetStatePath(statePath)
	if err := restarted.Init(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := restarted.Poll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 ||
		!reflect.DeepEqual(got[0].Added, []string{"auth_clawback_enabled"}) || got[0].OperationID != "101" ||
		!reflect.DeepEqual(got[1].Removed, []string{"auth_clawback_enabled"}) || got[1].OperationID != "102" {
		t.Fatalf("after restart got %+v, want clawback set at 101 and cleared at 102", got)
	}
	if err := restarted.Checkpoint(); err != nil {
		t.Fatal(err)
	}

	// Without a state path the same restart is blind to both.
	blind := NewWatcher(watchConfig(t), h)
	h.setOptions("103", []int{flagAuthClawbackEnabled}, nil)
	h.setOptions("104", nil, []int{flagAuthClawbackEnabled})
	if err := blind.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if ts, err := blind.Poll(ctx); err != nil || len(ts) != 0 {
		t.Fatalf("stateless restart got %+v, %v; want nothing (the documented blind spot)", ts, err)
	}

	// The checkpoint after the replay resumes at 102, so 103 and 104 replay
	// once and 101 and 102 are not reported again.
	again := NewWatcher(watchConfig(t), h)
	again.SetStatePath(statePath)
	if err := again.Init(ctx); err != nil {
		t.Fatal(err)
	}
	got, err = again.Poll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].OperationID != "103" || got[1].OperationID != "104" {
		t.Fatalf("second restart got %+v, want only 103 and 104", got)
	}
}

func TestWatcherStateFileIsAtomicAndValidated(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	statePath := filepath.Join(dir, "watch-state.json")
	h := &memHorizon{flags: horizon.Flags{AuthRequired: true}}
	h.setOptions("200", []int{flagAuthRevocable}, nil)

	w := NewWatcher(watchConfig(t), h)
	w.SetStatePath(statePath)
	if err := w.Init(ctx); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var s watchState
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	want := issuerCheckpoint{Cursor: "200", Flags: horizon.Flags{AuthRequired: true, AuthRevocable: true}}
	if s.Version != watchStateVersion || len(s.Issuers) != 1 || s.Issuers[watchedIssuer] != want {
		t.Fatalf("state = %+v, want version %d and %s at %+v", s, watchStateVersion, watchedIssuer, want)
	}
	// No temporary file is left beside the state.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("state directory holds %d entries, want only the state file", len(entries))
	}

	// A corrupt or unknown-version file is refused, not silently reset.
	for name, body := range map[string]string{
		"corrupt":         `{"version":1,"issuers":`,
		"unknown version": `{"version":2,"issuers":{}}`,
	} {
		if err := os.WriteFile(statePath, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		w := NewWatcher(watchConfig(t), h)
		w.SetStatePath(statePath)
		if err := w.Init(ctx); err == nil {
			t.Errorf("%s state file: Init succeeded, want an error", name)
		}
	}
}
