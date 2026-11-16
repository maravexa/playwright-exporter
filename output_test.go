package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	boundedOutputPrefix = "abcde"
	testRunnerCommand   = "custom-runner"
)

func TestBoundedOutput(t *testing.T) {
	for _, tc := range []struct {
		name      string
		want      string
		writes    []string
		limit     int
		truncated bool
	}{
		{name: "under limit", limit: 5, writes: []string{"ab", "cd"}, want: "abcd"},
		{name: "exact limit", limit: 5, writes: []string{boundedOutputPrefix, ""}, want: boundedOutputPrefix},
		{name: "one over", limit: 5, writes: []string{"abcdef"}, want: boundedOutputPrefix, truncated: true},
		{name: "multiple chunks", limit: 5, writes: []string{"ab", "cdef", "gh"}, want: boundedOutputPrefix, truncated: true},
		{name: "zero", limit: 0, writes: []string{"a"}, truncated: true},
		{name: "negative", limit: -1, writes: []string{"a"}, truncated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := boundedOutput{limit: tc.limit}
			for _, chunk := range tc.writes {
				n, err := output.Write([]byte(chunk))
				if n != len(chunk) || err != nil {
					t.Fatalf("Write(%q) = %d, %v", chunk, n, err)
				}
			}
			if string(output.Bytes()) != tc.want || output.truncated != tc.truncated {
				t.Fatalf("got %q, truncated=%v", output.Bytes(), output.truncated)
			}
			wantText := tc.want
			if tc.truncated {
				wantText += " [stderr truncated]"
			}
			if output.String() != wantText {
				t.Fatalf("String() = %q, want %q", output.String(), wantText)
			}
		})
	}
}

func TestBoundedOutputDrainsExcess(t *testing.T) {
	output := boundedOutput{limit: 5}
	n, err := io.Copy(&output, strings.NewReader(strings.Repeat("x", 10000)))
	if err != nil || n != 10000 || len(output.Bytes()) != 5 || !output.truncated {
		t.Fatalf("copy = %d, %v; captured=%d, truncated=%v", n, err, len(output.Bytes()), output.truncated)
	}
}

func TestExecutorRejectsOversizedReport(t *testing.T) {
	ex := &Executor{buildCmd: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, "cat")
		cmd.Stdin = io.MultiReader(strings.NewReader(`{"suites":[]}`), strings.NewReader(strings.Repeat(" ", maxReportBytes)))
		return cmd
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := ex.Run(ctx, &ScheduleConfig{Command: testRunnerCommand, PlaywrightDir: t.TempDir()})
	if result != nil || err == nil || !strings.Contains(err.Error(), "JSON output exceeds") {
		t.Fatalf("oversized JSON produced result=%v, error=%v", result, err)
	}
}

func TestExecutorBoundsStderrOnFailure(t *testing.T) {
	ex := &Executor{buildCmd: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, "sh", "-c", "cat >&2; exit 1")
		cmd.Stdin = strings.NewReader(strings.Repeat("x", maxStderrBytes+100))
		return cmd
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := ex.Run(ctx, &ScheduleConfig{Command: testRunnerCommand, PlaywrightDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "[stderr truncated]") {
		t.Fatalf("got %v", err)
	}
	if len(err.Error()) > maxStderrBytes+256 {
		t.Fatalf("stderr error grew to %d bytes", len(err.Error()))
	}
}

func TestExecutorAcceptsValidReportWithLargeStderr(t *testing.T) {
	ex := &Executor{buildCmd: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, "sh", "-c", `printf '{"suites":[]}'; cat >&2; exit 1`)
		cmd.Stdin = strings.NewReader(strings.Repeat("x", maxStderrBytes+100))
		return cmd
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := ex.Run(ctx, &ScheduleConfig{Command: testRunnerCommand, PlaywrightDir: t.TempDir()})
	if err != nil || result == nil {
		t.Fatalf("valid report on nonzero exit: result=%v, error=%v", result, err)
	}
}

func TestExecutorRejectsReportAfterTimeout(t *testing.T) {
	ex := &Executor{buildCmd: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", `printf '{"suites":[]}'; exec sleep 5`)
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	result, err := ex.Run(ctx, &ScheduleConfig{Command: testRunnerCommand, PlaywrightDir: t.TempDir()})
	if result != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout with valid JSON produced result=%v, error=%v", result, err)
	}
}

func TestExecutorDoesNotDiscardInheritedOutputOnNonzeroExit(t *testing.T) {
	ex := &Executor{buildCmd: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", `(sleep 2; printf ' trailing') & printf '{"suites":[]}'; exit 1`)
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := ex.Run(ctx, &ScheduleConfig{Command: testRunnerCommand, PlaywrightDir: t.TempDir()})
	if result != nil || err == nil {
		t.Fatalf("incomplete-looking report produced result=%v, error=%v", result, err)
	}
}

func TestCommandOutputCompleteReport(t *testing.T) {
	for _, exitCode := range []int{0, 1} {
		t.Run(fmt.Sprintf("exit %d", exitCode), func(t *testing.T) {
			output := newTestCommandOutput(t)
			cmd := exec.Command("sh", "-c", fmt.Sprintf(`printf '{"suites":[]}'; printf diagnostic >&2; exit %d`, exitCode))
			runErr, captureErr := output.run(cmd, time.Second)
			assertTestExitCode(t, runErr, exitCode)
			if captureErr != nil {
				t.Fatal(captureErr)
			}
			if string(output.stdout.output.Bytes()) != `{"suites":[]}` || output.stderr.output.String() != "diagnostic" {
				t.Fatalf("stdout=%q, stderr=%q", output.stdout.output.Bytes(), output.stderr.output.String())
			}
			assertTestCaptureFinished(t, output)
		})
	}
}

func TestCommandOutputBoundsInheritedPipes(t *testing.T) {
	for _, streams := range []string{"both", "stdout", "stderr"} {
		for _, exitCode := range []int{0, 1} {
			t.Run(fmt.Sprintf("%s exit %d", streams, exitCode), func(t *testing.T) {
				output := newTestCommandOutput(t)
				cmd, _, _ := inheritedOutputCommand(t, context.Background(), fmt.Sprint(exitCode), streams, "")
				start := time.Now()
				runErr, captureErr := output.run(cmd, 50*time.Millisecond)
				assertTestExitCode(t, runErr, exitCode)
				if !errors.Is(captureErr, errOutputDrainTimeout) {
					t.Fatalf("capture error=%v, want drain timeout", captureErr)
				}
				if elapsed := time.Since(start); elapsed > 2*time.Second {
					t.Fatalf("inherited pipes delayed return by %s", elapsed)
				}
				assertTestCaptureFinished(t, output)
			})
		}
	}
}

func TestExecutorRejectsInheritedPipes(t *testing.T) {
	for _, exitCode := range []string{"0", "1"} {
		t.Run("exit "+exitCode, func(t *testing.T) {
			ex := &Executor{buildCmd: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				cmd, _, _ := inheritedOutputCommand(t, ctx, exitCode, "both", "")
				return cmd
			}}
			start := time.Now()
			result, err := ex.Run(context.Background(), &ScheduleConfig{Command: testRunnerCommand, PlaywrightDir: t.TempDir()})
			if result != nil || !errors.Is(err, errOutputDrainTimeout) {
				t.Fatalf("held pipes with valid JSON produced result=%v, error=%v", result, err)
			}
			if elapsed := time.Since(start); elapsed > outputDrainTimeout+2*time.Second {
				t.Fatalf("inherited pipes delayed return by %s", elapsed)
			}
		})
	}
}

func TestCommandOutputCapturesDelayedTail(t *testing.T) {
	for _, exitCode := range []int{0, 1} {
		t.Run(fmt.Sprintf("exit %d", exitCode), func(t *testing.T) {
			output := newTestCommandOutput(t)
			cmd, release, _ := inheritedOutputCommand(t, context.Background(), fmt.Sprint(exitCode), "both", " trailing")
			// The descendant remains blocked until the direct process has exited.
			cmd.Stdout, cmd.Stderr = output.stdout.writer, output.stderr.writer
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			_ = output.stdout.writer.Close()
			_ = output.stderr.writer.Close()
			output.stdout.start()
			output.stderr.start()
			assertTestExitCode(t, cmd.Wait(), exitCode)
			release()
			if err := output.wait(time.Second); err != nil {
				t.Fatal(err)
			}
			if got := string(output.stdout.output.Bytes()); got != `{"suites":[]} trailing` {
				t.Fatalf("lost trailing output: %q", got)
			}
			if _, err := parseReport(output.stdout.output.Bytes()); err == nil {
				t.Fatal("trailing output must prevent accepting the valid JSON prefix")
			}
			assertTestCaptureFinished(t, output)
		})
	}
}

func TestExecutorCancellationWithInheritedPipes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd, _, ready := inheritedOutputCommand(t, ctx, "wait", "both", "")
	if err := ready.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	readyErr := make(chan error, 1)
	go func() {
		var signal [1]byte
		_, err := ready.Read(signal[:])
		readyErr <- err
		cancel()
	}()
	ex := &Executor{buildCmd: func(context.Context, string, ...string) *exec.Cmd { return cmd }}
	start := time.Now()
	result, err := ex.Run(ctx, &ScheduleConfig{Command: testRunnerCommand, PlaywrightDir: t.TempDir()})
	if readinessErr := <-readyErr; readinessErr != nil {
		t.Fatalf("waiting for direct process readiness: %v", readinessErr)
	}
	if result != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, errOutputDrainTimeout) {
		t.Fatalf("canceled command with inherited pipes produced result=%v, error=%v", result, err)
	}
	if elapsed := time.Since(start); elapsed > outputDrainTimeout+2*time.Second {
		t.Fatalf("cancellation delayed return by %s", elapsed)
	}
}

func TestCommandOutputPipeSetupFailure(t *testing.T) {
	for _, failOn := range []int{1, 2} {
		t.Run(fmt.Sprintf("pipe %d", failOn), func(t *testing.T) {
			setupErr := errors.New("pipe setup failed")
			var opened []*os.File
			calls := 0
			output, err := newCommandOutput(func() (*os.File, *os.File, error) {
				calls++
				if calls == failOn {
					return nil, nil, setupErr
				}
				reader, writer, err := os.Pipe()
				if err == nil {
					opened = append(opened, reader, writer)
				}
				return reader, writer, err
			})
			if output != nil || !errors.Is(err, setupErr) || calls != failOn {
				t.Fatalf("output=%v, error=%v, calls=%d", output, err, calls)
			}
			for _, file := range opened {
				assertTestFileClosed(t, file)
			}
		})
	}
}

func TestCommandOutputStartFailure(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(fmt.Sprintf("canceled %t", canceled), func(t *testing.T) {
			output := newTestCommandOutput(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cmd := exec.CommandContext(ctx, filepath.Join(t.TempDir(), "nonexistent-command"))
			if canceled {
				cancel()
				cmd = exec.CommandContext(ctx, "sh", "-c", "exit 0")
			}
			runErr, captureErr := output.run(cmd, time.Second)
			if runErr == nil || captureErr != nil {
				t.Fatalf("run error=%v, capture error=%v", runErr, captureErr)
			}
			if canceled && !errors.Is(runErr, context.Canceled) {
				t.Fatalf("run error=%v, want cancellation", runErr)
			}
			if output.stdout.done != nil || output.stderr.done != nil {
				t.Fatal("copy goroutines started despite command start failure")
			}
			for _, file := range []*os.File{output.stdout.reader, output.stdout.writer, output.stderr.reader, output.stderr.writer} {
				assertTestFileClosed(t, file)
			}
		})
	}
}

func TestCommandOutputKeepsCaptureErrorOnNonzeroExit(t *testing.T) {
	output := newTestCommandOutput(t)
	if err := output.stdout.reader.Close(); err != nil {
		t.Fatal(err)
	}
	runErr, captureErr := output.run(exec.Command("sh", "-c", "exit 1"), time.Second)
	assertTestExitCode(t, runErr, 1)
	if !errors.Is(captureErr, os.ErrClosed) || errors.Is(captureErr, errOutputDrainTimeout) {
		t.Fatalf("lost independent stdout read error: %v", captureErr)
	}
	assertTestCaptureFinished(t, output)
}

func newTestCommandOutput(t *testing.T) *commandOutput {
	t.Helper()
	output, err := newCommandOutput(os.Pipe)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(output.close)
	return output
}

func assertTestExitCode(t *testing.T, err error, want int) {
	t.Helper()
	if want == 0 {
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != want {
		t.Fatalf("run error=%v, want exit %d", err, want)
	}
}

func assertTestCaptureFinished(t *testing.T, output *commandOutput) {
	t.Helper()
	for _, pipe := range []*capturedPipe{&output.stdout, &output.stderr} {
		select {
		case <-pipe.done:
		default:
			t.Fatal("capture returned with a copy goroutine still active")
		}
		assertTestFileClosed(t, pipe.reader)
		assertTestFileClosed(t, pipe.writer)
	}
}

func assertTestFileClosed(t *testing.T, file *os.File) {
	t.Helper()
	if _, err := file.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Errorf("owned file %v remains open: %v", file, err)
		_ = file.Close()
	}
}

func inheritedOutputCommand(t *testing.T, ctx context.Context, exitCode, streams, tail string) (*exec.Cmd, func(), *os.File) {
	t.Helper()
	gateReader, gateWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	doneReader, doneWriter, err := os.Pipe()
	if err != nil {
		_ = gateReader.Close()
		_ = gateWriter.Close()
		t.Fatal(err)
	}
	readyReader, readyWriter, err := os.Pipe()
	if err != nil {
		_ = gateReader.Close()
		_ = gateWriter.Close()
		_ = doneReader.Close()
		_ = doneWriter.Close()
		t.Fatal(err)
	}
	redirect := ""
	switch streams {
	case "stdout":
		redirect = " 2>/dev/null"
	case "stderr":
		redirect = " >/dev/null"
	}
	// The descendant's lifetime is controlled by the test, not a long sleep.
	// A separate completion pipe lets cleanup wait for every inherited FD to close.
	script := `(read -r _ <&3; printf '%s' "$1"; printf done >&4)` + redirect + ` & printf '{"suites":[]}'; printf r >&5; if [ "$2" = wait ]; then exec sleep 5; fi; exit "$2"`
	cmd := exec.CommandContext(ctx, "sh", "-c", script, "capture-helper", tail, exitCode)
	cmd.ExtraFiles = []*os.File{gateReader, doneWriter, readyWriter}
	release := func() { _ = gateWriter.Close() }
	t.Cleanup(func() {
		release()
		_ = gateReader.Close()
		_ = doneWriter.Close()
		_ = readyReader.Close()
		_ = readyWriter.Close()
		defer func() { _ = doneReader.Close() }()
		if err := doneReader.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			t.Error(err)
			return
		}
		if _, err := io.ReadAll(doneReader); err != nil {
			t.Errorf("descendant did not finish after release: %v", err)
		}
	})
	return cmd, release, readyReader
}

func TestExecutorAcceptsCompleteReport(t *testing.T) {
	for _, exitCode := range []int{0, 1} {
		t.Run(fmt.Sprintf("exit %d", exitCode), func(t *testing.T) {
			ex := &Executor{buildCmd: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				return exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf(`printf '{"suites":[]}'; exit %d`, exitCode))
			}}
			result, err := ex.Run(context.Background(), &ScheduleConfig{Command: testRunnerCommand, PlaywrightDir: t.TempDir()})
			if result == nil || err != nil {
				t.Fatalf("complete report produced result=%v, error=%v", result, err)
			}
		})
	}
}

func TestCommandOutputAlreadyDrained(t *testing.T) {
	output := newTestCommandOutput(t)
	_ = output.stdout.writer.Close()
	_ = output.stderr.writer.Close()
	output.stdout.start()
	output.stderr.start()
	<-output.stdout.done
	<-output.stderr.done
	if err := output.wait(0); err != nil {
		t.Fatalf("completed output must not fail when timer is also ready: %v", err)
	}
	assertTestCaptureFinished(t, output)
}
