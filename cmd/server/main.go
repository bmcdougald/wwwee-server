// Command wwwee-server is a minimal, dependency-free static web server for
// hosting a department intranet site, designed to run as a hardened,
// non-root, read-only-root-filesystem container.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"wwwee-server/internal/config"
	"wwwee-server/internal/httpserver"
)

func main() {
	os.Exit(run())
}

func run() int {
	healthcheck := flag.Bool("healthcheck", false, "perform a local healthz check and exit (used as the container HEALTHCHECK)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
	}

	if *healthcheck {
		return runHealthcheck(cfg)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	handler := httpserver.New(cfg, logger)
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    cfg.MaxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	if cfg.TLSEnabled() {
		srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("starting server", "addr", cfg.Addr, "content_dir", cfg.ContentDir, "tls", cfg.TLSEnabled())
		if cfg.TLSEnabled() {
			serverErr <- srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			serverErr <- srv.ListenAndServe()
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			return 1
		}
	case <-ctx.Done():
		logger.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
			return 1
		}
	}

	logger.Info("server stopped")
	return 0
}

// runHealthcheck is invoked as `wwwee-server -healthcheck` from the
// container's HEALTHCHECK instruction. The distroless runtime image has no
// shell or curl, so the binary performs the probe itself.
func runHealthcheck(cfg config.Config) int {
	_, port, err := net.SplitHostPort(cfg.Addr)
	if err != nil {
		port = "8080"
	}
	scheme := "http"
	client := &http.Client{Timeout: 3 * time.Second}
	if cfg.TLSEnabled() {
		scheme = "https"
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec G402 -- loopback self-check only
		}
	}

	resp, err := client.Get(fmt.Sprintf("%s://127.0.0.1:%s/healthz", scheme, port))
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck failed:", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck failed: status", resp.StatusCode)
		return 1
	}
	return 0
}
