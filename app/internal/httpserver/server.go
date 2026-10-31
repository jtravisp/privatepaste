// Package httpserver wires routes, middleware and health endpoints into one
// http.Handler. Kept separate from main so it can be tested without a
// network listener or AWS.
package httpserver

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/jtravisp/privatepaste/internal/handler"
	"github.com/jtravisp/privatepaste/internal/store"
)

// contentSecurityPolicy allows only same-origin scripts, styles and fetches.
// The page is a crypto tool: an injected script could read the key from the
// URL fragment, so no inline script, no third-party origin, no framing.
const contentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; " +
	"img-src 'self' data:; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

type Server struct {
	store    store.Store
	frontend fs.FS
	ready    atomic.Bool
}

// New builds the server. frontend must contain index.html, app.js and
// style.css at its root.
func New(s store.Store, frontend fs.FS) *Server {
	srv := &Server{store: s, frontend: frontend}
	srv.ready.Store(true)
	return srv
}

// SetNotReady makes /readyz fail. Called on SIGTERM so Kubernetes removes the
// pod from Service endpoints before the listener closes.
func (s *Server) SetNotReady() { s.ready.Store(false) }

func (s *Server) Handler() http.Handler {
	pastes := handler.NewPasteHandler(s.store)
	mux := http.NewServeMux()

	mux.HandleFunc("POST /pastes", pastes.CreatePaste)
	mux.HandleFunc("GET /pastes/{id}", pastes.GetPaste)
	mux.HandleFunc("DELETE /pastes/{id}", pastes.DeletePaste)

	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /readyz", s.readyz)

	static := http.StripPrefix("/static/", http.FileServerFS(s.frontend))
	mux.Handle("GET /static/", cacheFor(5*time.Minute, static))
	// "/" and "/{id}" both serve the single page; app.js reads the path and
	// the #key fragment to decide what to show.
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /{id}", s.index)

	return recoverer(securityHeaders(logRequests(mux)))
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(s.frontend, "index.html")
	if err != nil {
		http.Error(w, "index.html missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if _, err := w.Write(data); err != nil {
		slog.Error("write index", "err", err)
	}
}

// healthz is the liveness probe: the process is up and serving HTTP. It
// deliberately does not check DynamoDB -- if the database is down,
// restarting the pod would not help, and failing liveness would turn a
// dependency outage into a crash loop.
func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("ok"))
}

// readyz is the readiness probe: this pod can serve a real request right
// now. It checks the table is reachable, and fails during shutdown.
func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.ready.Load() {
		http.Error(w, "shutting down", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		slog.Warn("readiness check failed", "err", err)
		http.Error(w, "database unreachable", http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ready"))
}

func cacheFor(d time.Duration, next http.Handler) http.Handler {
	value := "public, max-age=" + strconv.Itoa(int(d.Seconds()))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", value)
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		// The paste ID is in the path; never leak it to another site.
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Strict-Transport-Security", "max-age=31536000")
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// logRequests logs the route pattern ("GET /pastes/{id}"), not the raw path,
// so paste IDs do not end up in logs. Probes are skipped to keep logs quiet.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if r.Pattern == "GET /healthz" || r.Pattern == "GET /readyz" {
			return
		}
		slog.Info("request",
			"route", r.Pattern,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				slog.Error("panic", "value", v, "route", r.Pattern)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
