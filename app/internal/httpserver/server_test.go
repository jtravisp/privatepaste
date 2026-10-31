package httpserver

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jtravisp/privatepaste/internal/store"
)

var frontend = fstest.MapFS{
	"index.html": {Data: []byte("<html>index</html>")},
	"app.js":     {Data: []byte("// js")},
	"style.css":  {Data: []byte("/* css */")},
}

func do(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestRoutes(t *testing.T) {
	h := New(store.NewMemory(), frontend).Handler()
	cases := []struct {
		path     string
		status   int
		contains string
	}{
		{"/", 200, "index"},
		{"/AAAAAAAAAAAAAAAAAAAAA", 200, "index"},
		{"/static/app.js", 200, "// js"},
		{"/static/style.css", 200, "css"},
		{"/healthz", 200, "ok"},
		{"/readyz", 200, "ready"},
	}
	for _, c := range cases {
		rec := do(h, http.MethodGet, c.path)
		if rec.Code != c.status || !strings.Contains(rec.Body.String(), c.contains) {
			t.Errorf("GET %s: got %d %q", c.path, rec.Code, rec.Body.String())
		}
	}
}

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	h := New(store.NewMemory(), frontend).Handler()
	for _, path := range []string{"/", "/static/app.js", "/pastes/AAAAAAAAAAAAAAAAAAAAA"} {
		rec := do(h, http.MethodGet, path)
		csp := rec.Header().Get("Content-Security-Policy")
		if !strings.Contains(csp, "script-src 'self'") || strings.Contains(csp, "unsafe-inline") {
			t.Errorf("%s: weak or missing CSP %q", path, csp)
		}
		for _, hdr := range []string{"X-Content-Type-Options", "Referrer-Policy", "X-Frame-Options"} {
			if rec.Header().Get(hdr) == "" {
				t.Errorf("%s: missing %s", path, hdr)
			}
		}
	}
}

func TestReadyzFailsWhenDatabaseDown(t *testing.T) {
	mem := store.NewMemory()
	mem.PingErr = errors.New("unreachable")
	h := New(mem, frontend).Handler()
	if rec := do(h, http.MethodGet, "/readyz"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", rec.Code)
	}
	// Liveness must not depend on the database.
	if rec := do(h, http.MethodGet, "/healthz"); rec.Code != http.StatusOK {
		t.Fatalf("healthz got %d, want 200", rec.Code)
	}
}

func TestReadyzFailsDuringShutdown(t *testing.T) {
	srv := New(store.NewMemory(), frontend)
	srv.SetNotReady()
	if rec := do(srv.Handler(), http.MethodGet, "/readyz"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", rec.Code)
	}
}
