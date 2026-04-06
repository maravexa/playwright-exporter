package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	configPath := flag.String("config", "./config.yaml", "path to config YAML")
	printVersion := flag.Bool("version", false, "print version and exit")
	logLevel := flag.String("log.level", "info", "log level (debug, info, warn, error)")
	flag.Parse()

	if *printVersion {
		fmt.Printf("playwright-exporter %s (go%s)\n", version, runtime.Version())
		os.Exit(0)
	}

	// Configure structured logging.
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(*logLevel)); err != nil {
		slog.Error("invalid log level", "level", *logLevel, "error", err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})))

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		slog.Error("loading config", "error", err)
		os.Exit(1)
	}

	slog.Info("starting playwright-exporter",
		"version", version,
		"listen_address", cfg.ListenAddress,
		"schedules", len(cfg.Schedules),
	)
	for _, s := range cfg.Schedules {
		slog.Info("loaded schedule",
			"name", s.Name,
			"interval", s.Interval.Duration,
			"timeout", s.Timeout.Duration,
			"dir", s.PlaywrightDir,
		)
	}

	names := make([]string, len(cfg.Schedules))
	for i, s := range cfg.Schedules {
		names[i] = fmt.Sprintf("%s (%s)", s.Name, s.Interval.Duration)
	}

	// Set up Prometheus registry.
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	mc, err := NewMetricsCache(reg, cfg)
	if err != nil {
		slog.Error("initialising metrics", "error", err)
		os.Exit(1)
	}
	mc.SetBuildInfo(version)

	// Start one scheduler goroutine per schedule.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec := NewExecutor()
	for i := range cfg.Schedules {
		s := &cfg.Schedules[i]
		sched := NewScheduler(s, mc, exec)
		go sched.Run(ctx)
	}

	// HTTP server.
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, `<html><body><a href="/metrics">Metrics</a></body></html>`)
	})

	srv := &http.Server{
		Addr:    cfg.ListenAddress,
		Handler: mux,
	}

	// Start HTTP server in a goroutine.
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("HTTP server listening", "addr", cfg.ListenAddress)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Wait for shutdown signal or server error.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	select {
	case sig := <-quit:
		slog.Info("shutting down", "signal", sig)
	case err := <-serverErr:
		slog.Error("HTTP server error", "error", err)
	}

	cancel() // stop all schedulers

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	slog.Info("shutdown complete")
}
