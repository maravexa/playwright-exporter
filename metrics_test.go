package main

import (
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func newTestRegistry(t *testing.T, cfg *Config) (*MetricsCache, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	mc, err := NewMetricsCache(reg, cfg)
	if err != nil {
		t.Fatalf("NewMetricsCache: %v", err)
	}
	return mc, reg
}

func gatherGauge(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) float64 {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if labelsMatch(m.GetLabel(), labels) {
				return m.GetGauge().GetValue()
			}
		}
	}
	t.Fatalf("metric %q with labels %v not found", name, labels)
	return 0
}

func labelsMatch(pairs []*dto.LabelPair, want map[string]string) bool {
	got := make(map[string]string, len(pairs))
	for _, p := range pairs {
		got[p.GetName()] = p.GetValue()
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}

func minimalConfig(t *testing.T) *Config {
	t.Helper()
	dir := t.TempDir()
	return &Config{
		ListenAddress: ":9115",
		Schedules: []ScheduleConfig{
			{
				Name:          "sched",
				Interval:      Duration{10 * time.Minute},
				Timeout:       Duration{8 * time.Minute},
				PlaywrightDir: dir,
				Command:       "npx playwright test --reporter=json",
			},
		},
	}
}

func TestMetricsCache_InitialValues(t *testing.T) {
	cfg := minimalConfig(t)
	_, reg := newTestRegistry(t, cfg)

	v := gatherGauge(t, reg, "playwright_schedule_interval_seconds", map[string]string{"schedule": "sched"})
	if v != 600 {
		t.Errorf("schedule_interval_seconds: got %v, want 600", v)
	}
}

func TestMetricsCache_UpdateScheduleSuccess(t *testing.T) {
	cfg := minimalConfig(t)
	mc, reg := newTestRegistry(t, cfg)

	now := time.Now()
	result := &RunResult{
		Duration:   42 * time.Second,
		FinishedAt: now,
		Tests: []FlatTest{
			{Name: "Login > ok", Passed: true, DurationSeconds: 1.5},
			{Name: "Login > fail", Passed: false, DurationSeconds: 2.0},
		},
	}

	mc.UpdateScheduleSuccess(&cfg.Schedules[0], result)

	up := gatherGauge(t, reg, "playwright_up", map[string]string{"schedule": "sched"})
	if up != 1 {
		t.Errorf("playwright_up: got %v, want 1", up)
	}

	dur := gatherGauge(t, reg, "playwright_run_duration_seconds", map[string]string{"schedule": "sched"})
	if dur != 42 {
		t.Errorf("run_duration_seconds: got %v, want 42", dur)
	}

	ts := gatherGauge(t, reg, "playwright_last_run_timestamp_seconds", map[string]string{"schedule": "sched"})
	if ts != float64(now.Unix()) {
		t.Errorf("last_run_timestamp_seconds: got %v, want %v", ts, float64(now.Unix()))
	}

	okTest := gatherGauge(t, reg, "playwright_test_success", map[string]string{"schedule": "sched", "test": "Login > ok"})
	if okTest != 1 {
		t.Errorf("test_success[ok]: got %v, want 1", okTest)
	}

	failTest := gatherGauge(t, reg, "playwright_test_success", map[string]string{"schedule": "sched", "test": "Login > fail"})
	if failTest != 0 {
		t.Errorf("test_success[fail]: got %v, want 0", failTest)
	}
}

func TestMetricsCache_UpdateScheduleFailure(t *testing.T) {
	cfg := minimalConfig(t)
	mc, reg := newTestRegistry(t, cfg)

	// First a success to set up=1.
	mc.UpdateScheduleSuccess(&cfg.Schedules[0], &RunResult{
		Duration:   1 * time.Second,
		FinishedAt: time.Now(),
		Tests:      []FlatTest{{Name: "t", Passed: true}},
	})

	mc.UpdateScheduleFailure("sched")

	up := gatherGauge(t, reg, "playwright_up", map[string]string{"schedule": "sched"})
	if up != 0 {
		t.Errorf("playwright_up after failure: got %v, want 0", up)
	}
}

func TestMetricsCache_CustomLabels(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		ListenAddress: ":9115",
		Schedules: []ScheduleConfig{
			{
				Name:          "sched",
				Interval:      Duration{10 * time.Minute},
				Timeout:       Duration{8 * time.Minute},
				PlaywrightDir: dir,
				Command:       "npx playwright test --reporter=json",
				Labels:        map[string]string{"site": "example", "tier": "prod"},
			},
		},
	}
	mc, reg := newTestRegistry(t, cfg)

	mc.UpdateScheduleSuccess(&cfg.Schedules[0], &RunResult{
		Duration:   1 * time.Second,
		FinishedAt: time.Now(),
		Tests:      []FlatTest{{Name: "my test", Passed: true, DurationSeconds: 0.5}},
	})

	v := gatherGauge(t, reg, "playwright_test_success", map[string]string{
		"schedule": "sched",
		"test":     "my test",
		"site":     "example",
		"tier":     "prod",
	})
	if v != 1 {
		t.Errorf("test_success with custom labels: got %v, want 1", v)
	}
}

func TestMetricsCache_ConcurrentAccess(t *testing.T) {
	cfg := minimalConfig(t)
	mc, _ := newTestRegistry(t, cfg)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mc.UpdateScheduleSuccess(&cfg.Schedules[0], &RunResult{
				Duration:   time.Second,
				FinishedAt: time.Now(),
				Tests:      []FlatTest{{Name: "t", Passed: true}},
			})
		}()
	}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mc.UpdateScheduleFailure("sched")
		}()
	}
	wg.Wait()
}

func TestMetricsCache_IncrementSkipCounter(t *testing.T) {
	cfg := minimalConfig(t)
	mc, reg := newTestRegistry(t, cfg)

	mc.IncrementSkipCounter("sched")
	mc.IncrementSkipCounter("sched")

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range mfs {
		if mf.GetName() != "playwright_run_skip_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "schedule" && lp.GetValue() == "sched" {
					if v := m.GetCounter().GetValue(); v != 2 {
						t.Errorf("run_skip_total: got %v, want 2", v)
					}
					return
				}
			}
		}
	}
	t.Error("run_skip_total metric not found")
}
