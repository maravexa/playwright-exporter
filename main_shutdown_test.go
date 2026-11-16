//go:build linux

package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

const (
	shutdownHelperPort = "PLAYWRIGHT_EXPORTER_SHUTDOWN_HELPER_PORT"
	shutdownHelperMode = "PLAYWRIGHT_EXPORTER_SHUTDOWN_HELPER_MODE"
)

func TestExporterShutdownHelper(t *testing.T) {
	portText := os.Getenv(shutdownHelperPort)
	if portText == "" {
		return
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		t.Fatal("invalid helper readiness port")
	}
	mode := os.Getenv(shutdownHelperMode)
	interrupts := make(chan os.Signal, 1)
	switch mode {
	case cooperativeHelper:
		signal.Notify(interrupts, os.Interrupt)
		defer signal.Stop(interrupts)
	case ignoringHelper:
		signal.Ignore(os.Interrupt)
	default:
		t.Fatal("invalid shutdown helper mode")
	}

	// This watchdog also bounds an orphan if the exporter exits before cleanup.
	watchdog := time.AfterFunc(20*time.Second, func() { os.Exit(3) })
	defer watchdog.Stop()
	// #nosec G704 -- Only a validated numeric port comes from the environment; the destination is fixed to loopback.
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := fmt.Fprintln(conn, "ready"); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	if ready, err := reader.ReadString('\n'); err != nil || ready != "continue\n" {
		t.Fatalf("parent readiness: %q, %v", ready, err)
	}
	// The parent's connection owns fixture cleanup, even if the exporter exits early.
	go func() {
		_, _ = io.Copy(io.Discard, reader)
		os.Exit(0)
	}()
	if mode == ignoringHelper {
		time.Sleep(20 * time.Second)
		os.Exit(3)
	}
	<-interrupts
	if _, err := fmt.Fprintln(conn, "cleaned"); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

func TestExporterShutdownWaitsForSchedulers(t *testing.T) {
	buildDir := t.TempDir()
	binary := filepath.Join(buildDir, "playwright-exporter")
	buildCtx, buildCancel := context.WithTimeout(context.Background(), time.Minute)
	defer buildCancel()
	compiler, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	build := exec.CommandContext(buildCtx, compiler, "build", "-race", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build exporter: %v\n%s", err, output)
	}
	for _, mode := range []string{cooperativeHelper, ignoringHelper} {
		t.Run(mode, func(t *testing.T) {
			testExporterShutdown(t, binary, mode)
		})
	}
}

func testExporterShutdown(t *testing.T, binary, mode string) {
	t.Helper()
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if deadlineErr := listener.SetDeadline(time.Now().Add(10 * time.Second)); deadlineErr != nil {
		t.Fatal(deadlineErr)
	}

	dir := t.TempDir()
	helper, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if linkErr := os.Symlink(helper, filepath.Join(dir, "shutdown-helper")); linkErr != nil {
		t.Fatal(linkErr)
	}
	config := fmt.Sprintf(`listen_address: "127.0.0.1:0"
schedules:
  - name: shutdown
    interval: 1h
    timeout: 30m
    playwright_dir: %q
    command: "./shutdown-helper -test.run=^TestExporterShutdownHelper$"
`, dir)
	configPath := filepath.Join(dir, "config.yaml")
	if writeErr := os.WriteFile(configPath, []byte(config), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-config", configPath)
	cmd.Env = append(os.Environ(),
		shutdownHelperPort+"="+strconv.Itoa(listener.Addr().(*net.TCPAddr).Port),
		shutdownHelperMode+"="+mode,
	)
	cmd.WaitDelay = time.Second
	var logs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if startErr := cmd.Start(); startErr != nil {
		t.Fatal(startErr)
	}
	var waitErr error
	exited := make(chan struct{})
	go func() {
		waitErr = cmd.Wait()
		close(exited)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		select {
		case <-exited:
			if t.Failed() {
				t.Logf("exporter output:\n%s", logs.String())
			}
		case <-time.After(3 * time.Second):
			t.Error("exporter did not exit after fixture cleanup")
		}
	})

	conn, err := listener.AcceptTCP()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if deadlineErr := conn.SetDeadline(time.Now().Add(12 * time.Second)); deadlineErr != nil {
		t.Fatal(deadlineErr)
	}
	reader := bufio.NewReader(conn)
	if ready, readErr := reader.ReadString('\n'); readErr != nil || ready != "ready\n" {
		t.Fatalf("helper readiness: %q, %v", ready, readErr)
	}
	if _, err := fmt.Fprintln(conn, "continue"); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
		if waitErr != nil {
			t.Fatalf("exporter shutdown: %v", waitErr)
		}
	case <-time.After(9 * time.Second):
		t.Fatal("exporter shutdown exceeded its bounded cleanup budget")
	}
	elapsed := time.Since(start)
	if mode == ignoringHelper && elapsed < cancellationGracePeriod {
		t.Errorf("exporter exited after %s, before the %s cleanup grace", elapsed, cancellationGracePeriod)
	}
	if deadlineErr := conn.SetReadDeadline(time.Now().Add(time.Second)); deadlineErr != nil {
		t.Fatal(deadlineErr)
	}
	if mode == cooperativeHelper {
		if cleaned, err := reader.ReadString('\n'); err != nil || cleaned != "cleaned\n" {
			t.Errorf("cooperative cleanup: %q, %v", cleaned, err)
		}
	}
	if _, readErr := reader.ReadByte(); !errors.Is(readErr, io.EOF) {
		t.Errorf("helper control connection still open after exporter shutdown: %v", readErr)
	}
	t.Logf("exporter waited %s for %s helper cleanup", elapsed, mode)
}
