package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// PlaywrightReport represents the top-level JSON output from Playwright's JSON reporter.
type PlaywrightReport struct {
	Suites []Suite `json:"suites"`
	Stats  Stats   `json:"stats"`
}

// Suite is a named group of specs in a Playwright report.
type Suite struct {
	Title string `json:"title"`
	Specs []Spec `json:"specs"`
}

// Spec is a single test specification containing one or more test runs.
type Spec struct {
	Title string `json:"title"`
	Tests []Test `json:"tests"`
}

// Test holds the results of one spec execution.
type Test struct {
	Results []TestResult `json:"results"`
}

// TestResult is the outcome of a single test attempt.
type TestResult struct {
	Status   string  `json:"status"`   // "passed", "failed", "timedOut", "skipped"
	Duration float64 `json:"duration"` // milliseconds
}

// Stats holds aggregate statistics from a Playwright run.
type Stats struct {
	Expected   int     `json:"expected"`
	Unexpected int     `json:"unexpected"`
	Flaky      int     `json:"flaky"`
	Skipped    int     `json:"skipped"`
	Duration   float64 `json:"duration"` // milliseconds
}

// FlatTest is a parsed test result with a human-readable name.
type FlatTest struct {
	Name            string
	Passed          bool
	DurationSeconds float64
}

// RunResult is the outcome of executing one schedule's Playwright suite.
type RunResult struct {
	FinishedAt time.Time
	Tests      []FlatTest
	Duration   time.Duration
}

// Executor runs a Playwright command and parses its JSON output.
type Executor struct {
	// buildCmd is a hook for tests to supply a fake command builder.
	buildCmd func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// NewExecutor creates an Executor that runs real subprocesses.
func NewExecutor() *Executor {
	return &Executor{
		buildCmd: exec.CommandContext,
	}
}

// Run executes the schedule's Playwright command with the given context and
// returns a RunResult. If the command exits non-zero but produces valid JSON
// stdout, the JSON is still parsed — Playwright exits non-zero on test failures.
func (e *Executor) Run(ctx context.Context, s *ScheduleConfig) (*RunResult, error) {
	parts := strings.Fields(s.Command)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	cmd := e.buildCmd(ctx, parts[0], parts[1:]...)
	cmd.Dir = s.PlaywrightDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start)

	stdoutBytes := stdout.Bytes()

	// Attempt JSON parse regardless of exit code — Playwright exits non-zero
	// on test failures but still emits valid JSON.
	report, parseErr := parseReport(stdoutBytes)
	if parseErr != nil {
		stderrStr := stderr.String()
		if runErr != nil {
			slog.Error("playwright command failed", "error", runErr, "stderr", stderrStr)
			return nil, fmt.Errorf("command failed: %w; stderr: %s", runErr, stderrStr)
		}
		slog.Error("failed to parse playwright JSON output", "error", parseErr, "stderr", stderrStr)
		return nil, fmt.Errorf("parsing JSON output: %w", parseErr)
	}

	tests := flattenReport(report)

	return &RunResult{
		Tests:      tests,
		Duration:   elapsed,
		FinishedAt: time.Now(),
	}, nil
}

// ParsePlaywrightReport parses Playwright's JSON reporter output.
// It is the exported entry point for testing and fuzzing.
func ParsePlaywrightReport(data []byte) (*PlaywrightReport, error) {
	return parseReport(data)
}

// parseReport unmarshals Playwright's JSON reporter output.
func parseReport(data []byte) (*PlaywrightReport, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("empty stdout")
	}
	var report PlaywrightReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	return &report, nil
}

// flattenReport converts nested suites/specs/tests into a flat list of FlatTest,
// building names as "Suite > Spec".
func flattenReport(report *PlaywrightReport) []FlatTest {
	var tests []FlatTest
	for _, suite := range report.Suites {
		for _, spec := range suite.Specs {
			name := buildTestName(suite.Title, spec.Title)
			for _, test := range spec.Tests {
				result := lastResult(test.Results)
				tests = append(tests, FlatTest{
					Name:            name,
					Passed:          result.Status == "passed",
					DurationSeconds: result.Duration / 1000.0,
				})
			}
		}
	}
	return tests
}

// buildTestName joins suite and spec titles with " > ".
func buildTestName(suite, spec string) string {
	if suite == "" {
		return spec
	}
	if spec == "" {
		return suite
	}
	return suite + " > " + spec
}

// lastResult returns the last TestResult, or a zero value if the slice is empty.
func lastResult(results []TestResult) TestResult {
	if len(results) == 0 {
		return TestResult{}
	}
	return results[len(results)-1]
}
