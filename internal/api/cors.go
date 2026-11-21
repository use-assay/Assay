package api

import "net/http"

// withCORS wraps a handler with CORS headers for the read-only API surface.
//
// Policy (issue #428): GET /api/v1/* is public and intended to be called from
// browsers on any origin, so every response carries
// Access-Control-Allow-Origin: *. OPTIONS preflight is answered inline, before
// the request reaches the router, so no additional route registration is needed.
//
// The policy is intentionally permissive: the API is read-only and carries no
// credentials. Any header the browser sends is forwarded (Allow-Headers: *) and
// the methods are limited to GET and OPTIONS.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, X-Request-Id")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
