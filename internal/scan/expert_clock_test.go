package scan_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/use-assay/assay/internal/scan"
	"github.com/use-assay/assay/internal/sep1"
)

func TestExpertAssetEvidenceUsesScannerClockAndCanonicalFailure(t *testing.T) {
	sources := newFakeSources(t)
	fixed := time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		status int
	}{
		{"success", http.StatusOK},
		{"failure", http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expert := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.Contains(r.URL.Path, "/asset/") {
					w.WriteHeader(tc.status)
				}
				_, _ = w.Write([]byte("{}"))
			}))
			defer expert.Close()
			sc := scan.New()
			sc.Now = func() time.Time { return fixed }
			sc.Horizon.BaseURL = sources.horizon.URL
			sc.Expert.BaseURL = expert.URL
			sc.Toml.HTTP.Transport = &singleHostTransport{host: strings.TrimPrefix(sources.toml.URL, "http://")}
			sub, err := sc.Subject(context.Background(), mustParse(t, "USDC-"+scanIssuer))
			if err != nil {
				t.Fatal(err)
			}
			if !sub.ExpertAssetAttemptedAt.Equal(fixed) {
				t.Fatalf("attempt time = %v, want %v", sub.ExpertAssetAttemptedAt, fixed)
			}
			if tc.status == http.StatusOK {
				if !sub.ExpertAssetFetchedAt.Equal(fixed) || sub.ExpertAssetErr != "" {
					t.Fatalf("unexpected success evidence: fetched=%v error=%q", sub.ExpertAssetFetchedAt, sub.ExpertAssetErr)
				}
			} else if !sub.ExpertAssetFetchedAt.IsZero() || sub.ExpertAssetErr != sep1.FailureFetch {
				t.Fatalf("unexpected failure evidence: fetched=%v error=%q", sub.ExpertAssetFetchedAt, sub.ExpertAssetErr)
			}
		})
	}
}
