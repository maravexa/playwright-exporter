package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	cancellationHelperMode = "PLAYWRIGHT_EXPORTER_CANCEL_HELPER"
	cooperativeHelper      = "cooperate"
	ignoringHelper         = "ignore"
)

func TestRunnerCancellationHelper(_ *testing.T) {
	mode := os.Getenv(cancellationHelperMode)
	if mode == "" {
		return
	}
	interrupts := make(chan os.Signal, 1)
	if mode == cooperativeHelper {
		signal.Notify(interrupts, os.Interrupt)
	} else {
		signal.Ignore(os.Interrupt)
	}
	if _, err := fmt.Fprintln(os.Stdout, "ready"); err != nil {
		os.Exit(2)
	}
	if mode == cooperativeHelper {
		<-interrupts
		if _, err := fmt.Fprintln(os.Stdout, "cleaned"); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestRunnerCancellation(t *testing.T) {
	for _, mode := range []string{cooperativeHelper, ignoringHelper} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := newRunnerCommand(ctx, os.Args[0], "-test.run=^TestRunnerCancellationHelper$")
			cmd.Env = append(os.Environ(), cancellationHelperMode+"="+mode)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer stdout.Close()
			if startErr := cmd.Start(); startErr != nil {
				t.Fatal(startErr)
			}
			t.Cleanup(func() { _ = cmd.Process.Kill() })
			var watchdogFired atomic.Bool
			watchdog := time.AfterFunc(cancellationGracePeriod+5*time.Second, func() {
				watchdogFired.Store(true)
				_ = cmd.Process.Kill()
				_ = stdout.Close()
			})
			defer watchdog.Stop()
			reader := bufio.NewReader(stdout)
			ready, err := reader.ReadString('\n')
			if err != nil || ready != "ready\n" {
				cancel()
				_ = cmd.Wait()
				t.Fatalf("helper readiness: %q, %v", ready, err)
			}
			start := time.Now()
			cancel()
			remaining, readErr := io.ReadAll(reader)
			runErr := cmd.Wait()
			if watchdogFired.Load() || time.Since(start) > cancellationGracePeriod+3*time.Second {
				t.Fatal("cancellation exceeded its independent completion watchdog")
			}
			if ctx.Err() != context.Canceled || runErr == nil || readErr != nil {
				t.Fatalf("cancellation: context=%v, process=%v, capture=%v", ctx.Err(), runErr, readErr)
			}
			if mode == cooperativeHelper {
				if strings.TrimSpace(string(remaining)) != "cleaned" {
					t.Fatalf("interrupt cleanup did not finish: %q", remaining)
				}
			} else {
				if len(remaining) != 0 {
					t.Fatalf("ignoring helper unexpectedly cleaned up: %q", remaining)
				}
				if time.Since(start) < cancellationGracePeriod {
					t.Fatal("ignoring helper was killed before the allowed grace")
				}
			}
		})
	}
}
