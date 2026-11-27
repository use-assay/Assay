package main

import (
	"net/http"
	"testing"

	"github.com/use-assay/assay/internal/api"
)

func TestHTTPServerWriteTimeoutCoversHandlerDeadlines(t *testing.T) {
	srv := newHTTPServer(":0", http.NotFoundHandler())
	wantMinimum := api.DefaultExposureTimeout + serverWriteTimeoutMargin
	if srv.WriteTimeout < wantMinimum {
		t.Fatalf("WriteTimeout = %s, want at least %s", srv.WriteTimeout, wantMinimum)
	}
	if srv.WriteTimeout <= api.DefaultScanTimeout {
		t.Fatalf("WriteTimeout = %s, must exceed the scan deadline %s", srv.WriteTimeout, api.DefaultScanTimeout)
	}
}
