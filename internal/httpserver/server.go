// Package httpserver assembles the HTTP handlers for the intranet server:
// a hardened static file server plus liveness/readiness endpoints.
package httpserver

import (
	"log/slog"
	"net/http"

	"wwwee-server/internal/config"
)

// New builds the top-level http.Handler for the server, wiring the static
// content handler and health endpoints behind the shared hardening
// middleware stack.
func New(cfg config.Config, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/healthz", healthzHandler())
	mux.Handle("/readyz", readyzHandler(cfg.ContentDir))
	mux.Handle("/", newStaticHandler(cfg.ContentDir))

	var h http.Handler = mux
	h = securityHeaders(h)
	h = restrictMethods(h)
	h = requestLogger(logger, h)
	h = recoverer(logger, h)
	return h
}
