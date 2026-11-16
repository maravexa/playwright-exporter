//go:build linux

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const pidHelperMode = "PLAYWRIGHT_PID_HELPER_MODE"
const pidHelperFile = "PLAYWRIGHT_PID_HELPER_FILE"

type fixtureIdentity struct {
	PID       int
	StartTime string
}

func TestPIDNamespaceHelper(t *testing.T) {
	mode := os.Getenv(pidHelperMode)
	if mode == "" {
		return
	}
	time.AfterFunc(15*time.Second, func() { os.Exit(3) })
	if mode == "child" {
		// /proc is not remounted here: this captures the host-visible identity for
		// the external observer without adding mount privileges to the runner.
		data, err := os.ReadFile("/proc/self/stat")
		if err != nil {
			os.Exit(2)
		}
		pid, err := strconv.Atoi(strings.Fields(string(data))[0])
		if err != nil {
			os.Exit(2)
		}
		fields := strings.Fields(strings.SplitN(string(data), ")", 2)[1])
		identity, _ := json.Marshal(fixtureIdentity{PID: pid, StartTime: fields[19]})
		if os.WriteFile(os.Getenv(pidHelperFile), identity, 0600) != nil {
			os.Exit(2)
		}
		for {
			time.Sleep(time.Second)
		}
	}
	if os.Getpid() != 1 {
		os.Exit(2)
	}
	// #nosec G702 -- The gated fixture re-executes this same test binary, with fixed test arguments.
	child := exec.Command(os.Args[0], "-test.run=^TestPIDNamespaceHelper$")
	child.Env = append(os.Environ(), pidHelperMode+"=child")
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	if child.Start() != nil {
		os.Exit(2)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(os.Getenv(pidHelperFile)); err == nil {
			break
		}
		if time.Now().After(deadline) {
			os.Exit(2)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if mode == "success" {
		fmt.Print(`{"suites":[]}`)
		os.Exit(0)
	}
	for {
		time.Sleep(time.Second)
	}
}

func TestPIDNamespaceCleansSuccessfulAndTimedOutRuns(t *testing.T) {
	if os.Getenv("PLAYWRIGHT_APPROVED_PID_NAMESPACE_TEST") != "1" {
		t.Skip("requires separately approved Linux namespace-policy test; not a local clearance")
	}
	for _, mode := range []string{"success", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "identity.json")
			binary, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(binary, filepath.Join(dir, "helper")); err != nil {
				t.Fatal(err)
			}
			t.Setenv(pidHelperMode, mode)
			t.Setenv(pidHelperFile, path)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			executor := NewExecutor()
			executor.isolateProcessTree = true
			_, err = executor.Run(ctx, &ScheduleConfig{Command: "./helper -test.run=^TestPIDNamespaceHelper$", PlaywrightDir: dir})
			if mode == "success" && err != nil {
				t.Fatalf("isolated successful run: %v", err)
			}
			if mode == "timeout" && (err == nil || ctx.Err() != context.DeadlineExceeded) {
				t.Fatalf("expected timeout: %v", err)
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("fixture was not launched: %v (run: %v)", readErr, err)
			}
			var identity fixtureIdentity
			if err := json.Unmarshal(data, &identity); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(3 * time.Second)
			for {
				stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", identity.PID))
				if os.IsNotExist(err) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				fields := strings.Fields(strings.SplitN(string(stat), ")", 2)[1])
				if fields[19] != identity.StartTime {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("recorded descendant identity remains after namespace init exit")
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}
