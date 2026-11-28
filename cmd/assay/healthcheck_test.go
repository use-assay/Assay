package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthcheckAcceptsLiveness(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	if err := runHealthcheck([]string{"-url", srv.URL + "/healthz"}); err != nil {
		t.Fatalf("healthcheck: %v", err)
	}
}

func TestHealthcheckRejectsBadAnswers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"non-200 status", http.StatusServiceUnavailable, "ok"},
		{"unexpected body", http.StatusOK, "healthy"},
		{"empty body", http.StatusOK, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			if err := runHealthcheck([]string{"-url", srv.URL + "/healthz"}); err == nil {
				t.Fatal("healthcheck accepted an unhealthy answer")
			}
		})
	}
}

func TestHealthcheckReportsConnectionFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening now

	if err := runHealthcheck([]string{"-url", url + "/healthz"}); err == nil {
		t.Fatal("healthcheck accepted an unreachable endpoint")
	}
}
