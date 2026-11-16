package main

import (
	"os"
	"path/filepath"
	"slices"
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

func TestFlattenReport_NestedSuites(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "nested.json"))
	if err != nil {
		t.Fatal(err)
	}
	report, err := parseReport(data)
	if err != nil {
		t.Fatal(err)
	}

	want := []FlatTest{
		{Name: "checkout.spec.ts > opens storefront", Passed: true, DurationSeconds: 0.125},
		{Name: "checkout.spec.ts > Checkout > submits order", Passed: true, DurationSeconds: 0.45},
		{Name: "checkout.spec.ts > Checkout > Card > declines payment", DurationSeconds: 0.3},
		{Name: "checkout.spec.ts > Checkout > Card > awaits confirmation", DurationSeconds: 1},
		{Name: "checkout.spec.ts > Checkout > Card > missing result"},
		{Name: "checkout.spec.ts > Account > submits order"},
		{Name: "Login > enters credentials", Passed: true, DurationSeconds: 0.2},
	}
	if got := flattenReport(report); !slices.Equal(got, want) {
		t.Errorf("flattenReport() = %+v, want %+v", got, want)
	}
}

func TestFlattenReport_EmptyTitles(t *testing.T) {
	tests := []struct {
		name, parent, child, spec, want string
	}{
		{name: "empty parent", child: "Child", spec: "Spec", want: "Child > Spec"},
		{name: "empty child", parent: "Parent", spec: "Spec", want: "Parent > Spec"},
		{name: "empty spec", parent: "Parent", child: "Child", want: "Parent > Child"},
		{name: "all empty"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			report := &PlaywrightReport{Suites: []Suite{{
				Title: tc.parent,
				Suites: []Suite{{
					Title: tc.child,
					Specs: []Spec{{
						Title: tc.spec,
						Tests: []Test{{}},
					}},
				}},
			}}}
			want := []FlatTest{{Name: tc.want}}
			if got := flattenReport(report); !slices.Equal(got, want) {
				t.Errorf("flattenReport() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestFlattenReport_EmptySuites(t *testing.T) {
	for _, data := range []string{
		`{}`,
		`{"suites":null}`,
		`{"suites":[]}`,
		`{"suites":[{"title":"empty","suites":[{}, {"specs":[{"tests":[]}]}]}]}`,
	} {
		t.Run(data, func(t *testing.T) {
			report, err := parseReport([]byte(data))
			if err != nil {
				t.Fatal(err)
			}
			if got := flattenReport(report); len(got) != 0 {
				t.Errorf("flattenReport() = %+v, want no tests", got)
			}
		})
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
	nestedJSON, err := os.ReadFile("testdata/nested.json")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(nestedJSON)
	f.Add([]byte(`{"suites":[{"title":"outer","suites":[{"specs":[{"tests":[{}]}]}]}]}`))
	// Seed with minimal valid JSON
	f.Add([]byte(`{"suites":[]}`))
	// Seed with empty
	f.Add([]byte(""))
	// Seed with garbage
	f.Add([]byte("not json at all"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic. Errors are expected and fine.
		report, err := ParsePlaywrightReport(data)
		if err == nil {
			_ = flattenReport(report)
		}
	})
}
