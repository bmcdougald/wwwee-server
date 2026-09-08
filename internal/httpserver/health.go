package httpserver

import (
	"net/http"
	"os"
)

// healthzHandler reports liveness: the process is up and able to serve
// HTTP requests. It performs no I/O so it cannot be starved by a slow or
// unavailable content volume.
func healthzHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

// readyzHandler reports readiness: the configured content directory is
// present and readable, so the server can actually serve the site.
func readyzHandler(contentDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		info, err := os.Stat(contentDir)
		if err != nil || !info.IsDir() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})
}
