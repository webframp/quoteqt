package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/honeycombio/otel-config-go/otelconfig"
	"github.com/webframp/quoteqt/srv"
)

var flagListenAddr = flag.String("listen", ":8000", "address to listen on")

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	flag.Parse()
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// Initialize OpenTelemetry with Honeycomb
	// Requires HONEYCOMB_API_KEY environment variable
	honeycombKey := os.Getenv("HONEYCOMB_API_KEY")
	var shutdownOtel func()
	if honeycombKey != "" {
		shutdownOtel, err = otelconfig.ConfigureOpenTelemetry(
			otelconfig.WithServiceName("quoteqt"),
			otelconfig.WithServiceVersion(srv.Version),
			otelconfig.WithMetricsEnabled(false),
			otelconfig.WithExporterEndpoint("api.honeycomb.io:443"),
			otelconfig.WithHeaders(map[string]string{
				"x-honeycomb-team": honeycombKey,
			}),
		)
	} else {
		slog.Info("HONEYCOMB_API_KEY not set, tracing disabled")
	}
	if err != nil {
		slog.Warn("failed to configure OpenTelemetry", "error", err)
		// Continue without tracing - don't fail startup
	} else if shutdownOtel != nil {
		defer shutdownOtel()
		slog.Info("OpenTelemetry configured", "endpoint", "api.honeycomb.io:443")
	}

	// Load config from environment with defaults
	cfg := srv.ConfigFromEnv()
	// Only use os.Hostname() if HOSTNAME env var not set
	if cfg.Hostname == "localhost" {
		cfg.Hostname = hostname
	}

	// Parse admin emails from environment variable (comma-separated)
	if adminEnv := os.Getenv("ADMIN_EMAILS"); adminEnv != "" {
		for _, email := range strings.Split(adminEnv, ",") {
			if e := strings.TrimSpace(email); e != "" {
				cfg.AdminEmails = append(cfg.AdminEmails, e)
			}
		}
		slog.Info("admin emails configured", "count", len(cfg.AdminEmails))
	} else {
		slog.Warn("ADMIN_EMAILS not set, no admin access configured")
	}

	slog.Info("server config loaded",
		"api_rate_limit", cfg.APIRateLimit,
		"api_rate_interval", cfg.APIRateInterval,
		"api_rate_burst", cfg.APIRateBurst,
		"suggestion_rate_limit", cfg.SuggestionRateLimit,
		"suggestion_rate_interval", cfg.SuggestionRateInterval,
	)

	server, err := srv.NewWithConfig(cfg)
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}

	// Channel to receive shutdown signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Channel to receive server errors
	serverErr := make(chan error, 1)

	go func() {
		serverErr <- server.Serve(*flagListenAddr)
	}()

	// Wait for shutdown signal or server error
	select {
	case err := <-serverErr:
		return err
	case sig := <-stop:
		slog.Info("shutdown signal received", "signal", sig)
	}

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	slog.Info("server stopped gracefully")
	return nil
}
