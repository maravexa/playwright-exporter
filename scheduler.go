package main

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

// Scheduler manages the run loop for a single schedule.
type Scheduler struct {
	cfg     *ScheduleConfig
	metrics *MetricsCache
	exec    *Executor
	running atomic.Bool
}

// NewScheduler creates a Scheduler for the given schedule configuration.
func NewScheduler(cfg *ScheduleConfig, metrics *MetricsCache, exec *Executor) *Scheduler {
	return &Scheduler{
		cfg:     cfg,
		metrics: metrics,
		exec:    exec,
	}
}

// Run starts the scheduler loop. It executes the test suite immediately on
// startup, then on each tick of the configured interval. Run blocks until ctx
// is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	// Run immediately at startup.
	s.runOnce(ctx)

	ticker := time.NewTicker(s.cfg.Interval.Duration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runOnce(ctx)
		}
	}
}

// runOnce performs a single test execution, respecting the atomic running flag.
func (s *Scheduler) runOnce(ctx context.Context) {
	if !s.running.CompareAndSwap(false, true) {
		slog.Warn("skipping run, previous still in progress", "schedule", s.cfg.Name)
		s.metrics.IncrementSkipCounter(s.cfg.Name)
		return
	}
	defer s.running.Store(false)

	slog.Info("starting test run", "schedule", s.cfg.Name)

	runCtx, cancel := context.WithTimeout(ctx, s.cfg.Timeout.Duration)
	defer cancel()

	result, err := s.exec.Run(runCtx, s.cfg)
	if err != nil {
		slog.Error("test run failed", "schedule", s.cfg.Name, "error", err)
		s.metrics.UpdateScheduleFailure(s.cfg.Name)
		return
	}

	passed, failed := countResults(result.Tests)
	slog.Info("test run completed",
		"schedule", s.cfg.Name,
		"duration", result.Duration,
		"tests_passed", passed,
		"tests_failed", failed,
	)

	s.metrics.UpdateScheduleSuccess(s.cfg, result)
}

// countResults returns the number of passed and failed tests.
func countResults(tests []FlatTest) (passed, failed int) {
	for _, t := range tests {
		if t.Passed {
			passed++
		} else {
			failed++
		}
	}
	return
}
