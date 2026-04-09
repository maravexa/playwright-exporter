package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseReport_Success(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "success.json"))
	if err != nil {
		t.Fatal(err)
	}

	report, err := parseReport(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(report.Suites) != 2 {
		t.Errorf("expected 2 suites, got %d", len(report.Suites))
	}
	if report.Stats.Expected != 4 {
		t.Errorf("expected stats.expected=4, got %d", report.Stats.Expected)
	}
}

func TestParseReport_Failure(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "failure.json"))
	if err != nil {
		t.Fatal(err)
	}

	report, err := parseReport(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(report.Suites) != 2 {
		t.Errorf("expected 2 suites, got %d", len(report.Suites))
	}
	if report.Stats.Unexpected != 2 {
		t.Errorf("expected stats.unexpected=2, got %d", report.Stats.Unexpected)
	}
}

func TestParseReport_MalformedJSON(t *testing.T) {
	_, err := parseReport([]byte(`{not valid json`))
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestParseReport_EmptyStdout(t *testing.T) {
	_, err := parseReport([]byte(""))
	if err == nil {
		t.Error("expected error for empty stdout")
	}
	_, err = parseReport([]byte("   \n  "))
	if err == nil {
		t.Error("expected error for whitespace-only stdout")
	}
}

func TestFlattenReport_Success(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "success.json"))
	if err != nil {
		t.Fatal(err)
	}

	report, err := parseReport(data)
	if err != nil {
		t.Fatal(err)
	}

	tests := flattenReport(report)
	if len(tests) != 4 {
		t.Fatalf("expected 4 flat tests, got %d", len(tests))
	}

	// First test: Login > enters credentials successfully
	if tests[0].Name != "Login > enters credentials successfully" {
		t.Errorf("test[0].Name: got %q", tests[0].Name)
	}
	if !tests[0].Passed {
		t.Error("test[0] should be passed")
	}
	if tests[0].DurationSeconds != 3.42 {
		t.Errorf("test[0].DurationSeconds: got %v, want 3.42", tests[0].DurationSeconds)
	}
}

func TestFlattenReport_Failure(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "failure.json"))
	if err != nil {
		t.Fatal(err)
	}

	report, err := parseReport(data)
	if err != nil {
		t.Fatal(err)
	}

	tests := flattenReport(report)
	if len(tests) != 4 {
		t.Fatalf("expected 4 flat tests, got %d", len(tests))
	}

	// Check that failed test is marked not passed.
	failed := tests[1] // "Login > shows error on bad credentials"
	if failed.Name != "Login > shows error on bad credentials" {
		t.Errorf("unexpected name: %q", failed.Name)
	}
	if failed.Passed {
		t.Error("failed test should not be marked as passed")
	}

	// Check timedOut test.
	timedOut := tests[2] // "Dashboard > loads widgets"
	if timedOut.Passed {
		t.Error("timedOut test should not be marked as passed")
	}

	// Check skipped test.
	skipped := tests[3]
	if skipped.Passed {
		t.Error("skipped test should not be marked as passed")
	}
}

func TestBuildTestName(t *testing.T) {
	tests := []struct {
		suite, spec, want string
	}{
		{"Login", "enters credentials", "Login > enters credentials"},
		{"", "spec only", "spec only"},
		{"suite only", "", "suite only"},
	}
	for _, tc := range tests {
		got := buildTestName(tc.suite, tc.spec)
		if got != tc.want {
			t.Errorf("buildTestName(%q, %q) = %q, want %q", tc.suite, tc.spec, got, tc.want)
		}
	}
}

func TestLastResult_Empty(t *testing.T) {
	r := lastResult(nil)
	if r.Status != "" {
		t.Errorf("expected empty status, got %q", r.Status)
	}
}

func TestLastResult_Multiple(t *testing.T) {
	results := []TestResult{
		{Status: "failed", Duration: 100},
		{Status: "passed", Duration: 200},
	}
	r := lastResult(results)
	if r.Status != "passed" {
		t.Errorf("expected last result status=passed, got %q", r.Status)
	}
}

func FuzzParsePlaywrightReport(f *testing.F) {
	// Seed with valid report
	successJSON, err := os.ReadFile("testdata/success.json")
	if err == nil {
		f.Add(successJSON)
	}
	// Seed with failure report
	failJSON, err := os.ReadFile("testdata/failure.json")
	if err == nil {
		f.Add(failJSON)
	}
	// Seed with minimal valid JSON
	f.Add([]byte(`{"suites":[]}`))
	// Seed with empty
	f.Add([]byte(""))
	// Seed with garbage
	f.Add([]byte("not json at all"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic. Errors are expected and fine.
		_, _ = ParsePlaywrightReport(data)
	})
}
