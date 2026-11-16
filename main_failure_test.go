//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestExporterSingleScheduleFailureExitsNonzero(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "exporter")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	config := "listen_address: '127.0.0.1:0'\nrestart_on_run_failure: true\nschedules:\n  - name: isolated\n    interval: 1h\n    timeout: 1s\n    playwright_dir: '" + dir + "'\n    command: '/bin/false'\n"
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	runCtx, runCancel := context.WithTimeout(ctx, 10*time.Second)
	defer runCancel()
	cmd := exec.CommandContext(runCtx, binary, "-config", path)
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if runCtx.Err() != nil {
		t.Fatalf("process did not stop: %s", out)
	}
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("exit=%v, expected1: %s", err, out)
	}
}
