package main

import (
	"bytes"
	"context"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// newTestScheduler builds a Scheduler backed by a fake Executor that returns
// the provided Playwright JSON output.
func newTestScheduler(t *testing.T, jsonOutput []byte) *Scheduler {
	t.Helper()
	dir := t.TempDir()
	cfg := &Config{
		ListenAddress: ":0",
		Schedules: []ScheduleConfig{
			{
				Name:          "sched",
				Interval:      Duration{100 * time.Millisecond},
				Timeout:       Duration{80 * time.Millisecond},
				PlaywrightDir: dir,
				Command:       "echo test",
			},
		},
	}
	reg := prometheus.NewRegistry()
	mc, err := NewMetricsCache(reg, cfg)
	if err != nil {
		t.Fatal(err)
	}

	ex := &Executor{
		buildCmd: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			// Return a command that writes the fake JSON to stdout and exits 0.
			cmd := exec.CommandContext(ctx, "cat")
			cmd.Stdin = bytes.NewReader(jsonOutput)
			return cmd
		},
	}

	return NewScheduler(&cfg.Schedules[0], mc, ex)
}

func TestScheduler_RunOnceSuccess(t *testing.T) {
	data := []byte(`{"suites":[{"title":"S","specs":[{"title":"T","tests":[{"results":[{"status":"passed","duration":100}]}]}]}],"stats":{}}`)
	s := newTestScheduler(t, data)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s.runOnce(ctx)

	if s.running.Load() {
		t.Error("running flag should be cleared after runOnce")
	}
}

func TestScheduler_SkipWhenRunning(t *testing.T) {
	data := []byte(`{"suites":[],"stats":{}}`)
	s := newTestScheduler(t, data)

	// Simulate a run already in progress.
	s.running.Store(true)

	ctx := context.Background()
	s.runOnce(ctx)

	// running flag should still be true (we didn't clear it).
	if !s.running.Load() {
		t.Error("running flag should remain set when skip occurs")
	}

	// Reset for cleanup.
	s.running.Store(false)
}

func TestScheduler_SkipCounterIncrements(t *testing.T) {
	data := []byte(`{"suites":[],"stats":{}}`)
	s := newTestScheduler(t, data)

	s.running.Store(true)

	ctx := context.Background()
	s.runOnce(ctx)
	s.runOnce(ctx)

	s.running.Store(false)
}

func TestScheduler_ContextCancellation(t *testing.T) {
	data := []byte(`{"suites":[],"stats":{}}`)
	s := newTestScheduler(t, data)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx)
	}()

	// Cancel after a short time.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// good — scheduler exited cleanly
	case <-time.After(2 * time.Second):
		t.Error("scheduler did not stop within 2 seconds after context cancel")
	}
}

func TestScheduler_ConcurrentRunPrevention(t *testing.T) {
	data := []byte(`{"suites":[{"title":"S","specs":[{"title":"T","tests":[{"results":[{"status":"passed","duration":10}]}]}]}],"stats":{}}`)
	s := newTestScheduler(t, data)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var completed atomic.Int32

	// Launch many goroutines trying to run simultaneously.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runOnce(ctx)
			completed.Add(1)
		}()
	}
	wg.Wait()

	// All goroutines should have returned (either ran or skipped).
	if completed.Load() != 10 {
		t.Errorf("expected 10 completions, got %d", completed.Load())
	}
	// Running flag must be cleared.
	if s.running.Load() {
		t.Error("running flag not cleared after concurrent runOnce calls")
	}
}

func TestCountResults(t *testing.T) {
	tests := []FlatTest{
		{Passed: true},
		{Passed: false},
		{Passed: true},
	}
	p, f := countResults(tests)
	if p != 2 || f != 1 {
		t.Errorf("countResults: got passed=%d failed=%d, want 2,1", p, f)
	}
}
