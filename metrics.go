package main

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricsCache holds Prometheus metrics and provides synchronized access.
type MetricsCache struct {
	up               *prometheus.GaugeVec
	lastRunTimestamp *prometheus.GaugeVec
	runDuration      *prometheus.GaugeVec
	scheduleInterval *prometheus.GaugeVec
	runSkipTotal     *prometheus.CounterVec
	testSuccess      *prometheus.GaugeVec
	testDuration     *prometheus.GaugeVec
	buildInfo        *prometheus.GaugeVec
	// customLabelKeys maps schedule name → label key list
	customLabelKeys map[string][]string
	mu              sync.RWMutex
}

// NewMetricsCache creates and registers all Prometheus metrics.
// It returns an error if registration fails.
func NewMetricsCache(reg prometheus.Registerer, cfg *Config) (*MetricsCache, error) {
	mc := &MetricsCache{
		customLabelKeys: make(map[string][]string, len(cfg.Schedules)),
	}

	// Collect custom label key sets per schedule for test-level metrics.
	// All schedules can have different custom label sets, but for GaugeVec we
	// need a single unified label set.  We use a per-schedule metric approach:
	// one GaugeVec per schedule for test-level metrics so label sets stay clean.
	// For schedule-level metrics we use a shared GaugeVec with only "schedule".

	mc.up = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "playwright_up",
		Help: "Whether the last test run for this schedule completed without exporter-level error (1 = success, 0 = failure). A test failure is NOT an exporter failure.",
	}, []string{"schedule"})

	mc.lastRunTimestamp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "playwright_last_run_timestamp_seconds",
		Help: "Unix timestamp of the last successful run completion for this schedule.",
	}, []string{"schedule"})

	mc.runDuration = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "playwright_run_duration_seconds",
		Help: "Total wall-clock duration of the last test suite execution for this schedule.",
	}, []string{"schedule"})

	mc.scheduleInterval = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "playwright_schedule_interval_seconds",
		Help: "Configured polling interval for this schedule. Used for self-describing staleness alerting.",
	}, []string{"schedule"})

	mc.runSkipTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "playwright_run_skip_total",
		Help: "Total number of skipped runs because the previous run was still in progress.",
	}, []string{"schedule"})

	// Test-level metrics: we build a unified label set (schedule + test + all
	// custom labels across all schedules) so a single GaugeVec can serve all
	// schedules.  Labels absent for a given schedule get empty string values,
	// which is the standard Prometheus convention.
	customKeySet := make(map[string]bool)
	for _, s := range cfg.Schedules {
		for k := range s.Labels {
			customKeySet[k] = true
		}
	}
	customKeys := make([]string, 0, len(customKeySet))
	for k := range customKeySet {
		customKeys = append(customKeys, k)
	}
	testLabels := append([]string{"schedule", "test"}, customKeys...)

	mc.testSuccess = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "playwright_test_success",
		Help: "Whether the test passed on its last run (1 = passed, 0 = failed).",
	}, testLabels)

	mc.testDuration = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "playwright_test_duration_seconds",
		Help: "Duration of the test in seconds on its last run.",
	}, testLabels)

	mc.buildInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "playwright_exporter_build_info",
		Help: "Build information about the playwright-exporter.",
	}, []string{"version", "goversion"})

	collectors := []prometheus.Collector{
		mc.up,
		mc.lastRunTimestamp,
		mc.runDuration,
		mc.scheduleInterval,
		mc.runSkipTotal,
		mc.testSuccess,
		mc.testDuration,
		mc.buildInfo,
	}
	for _, c := range collectors {
		if err := reg.Register(c); err != nil {
			return nil, fmt.Errorf("registering metric: %w", err)
		}
	}

	// Pre-initialise schedule-level metrics so they exist from startup.
	for _, s := range cfg.Schedules {
		mc.scheduleInterval.WithLabelValues(s.Name).Set(s.Interval.Seconds())
		mc.up.WithLabelValues(s.Name) // initialise to 0
		mc.runSkipTotal.WithLabelValues(s.Name) // initialise counter to 0
	}

	// Store custom key list per schedule.
	for _, s := range cfg.Schedules {
		keys := make([]string, 0, len(s.Labels))
		for k := range s.Labels {
			keys = append(keys, k)
		}
		mc.customLabelKeys[s.Name] = keys
	}

	// Store global custom key ordering for test-level label building.
	mc.customLabelKeys["__all__"] = customKeys

	return mc, nil
}

// SetBuildInfo sets the build info metric.
func (mc *MetricsCache) SetBuildInfo(ver string) {
	mc.buildInfo.WithLabelValues(ver, runtime.Version()).Set(1)
}

// UpdateScheduleSuccess updates all metrics after a successful run.
func (mc *MetricsCache) UpdateScheduleSuccess(s *ScheduleConfig, result *RunResult) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.up.WithLabelValues(s.Name).Set(1)
	mc.lastRunTimestamp.WithLabelValues(s.Name).Set(float64(result.FinishedAt.Unix()))
	mc.runDuration.WithLabelValues(s.Name).Set(result.Duration.Seconds())

	allCustomKeys := mc.customLabelKeys["__all__"]

	for _, tr := range result.Tests {
		lblVals := make([]string, 0, 2+len(allCustomKeys))
		lblVals = append(lblVals, s.Name, tr.Name)
		for _, k := range allCustomKeys {
			lblVals = append(lblVals, s.Labels[k]) // empty string if absent
		}
		success := 0.0
		if tr.Passed {
			success = 1.0
		}
		mc.testSuccess.WithLabelValues(lblVals...).Set(success)
		mc.testDuration.WithLabelValues(lblVals...).Set(tr.DurationSeconds)
	}
}

// UpdateScheduleFailure marks a schedule as down without touching test-level metrics.
func (mc *MetricsCache) UpdateScheduleFailure(scheduleName string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.up.WithLabelValues(scheduleName).Set(0)
}

// IncrementSkipCounter increments the skip counter for a schedule.
func (mc *MetricsCache) IncrementSkipCounter(scheduleName string) {
	// CounterVec is safe to call without the cache mutex.
	mc.runSkipTotal.WithLabelValues(scheduleName).Inc()
}
