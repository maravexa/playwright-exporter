//go:build linux

package main

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestProcessTreeIsolationAttributes(t *testing.T) {
	attr, err := processTreeIsolation()
	if err != nil {
		t.Fatal(err)
	}
	if attr.Cloneflags != syscall.CLONE_NEWUSER|syscall.CLONE_NEWPID {
		t.Fatalf("unexpected namespace flags: %x", attr.Cloneflags)
	}
	if len(attr.UidMappings) != 1 || attr.UidMappings[0] != (syscall.SysProcIDMap{ContainerID: os.Getuid(), HostID: os.Getuid(), Size: 1}) {
		t.Fatal("UID mapping not limited to service identity")
	}
	if len(attr.GidMappings) != 1 || attr.GidMappings[0] != (syscall.SysProcIDMap{ContainerID: os.Getgid(), HostID: os.Getgid(), Size: 1}) || attr.GidMappingsEnableSetgroups {
		t.Fatal("GID mapping not restricted")
	}
	if attr.Credential != nil || attr.UseCgroupFD {
		t.Fatal("unexpected credential or cgroup-manager authority")
	}
}

func TestIsolationRequestHasNoUnisolatedRetry(t *testing.T) {
	var cmd *exec.Cmd
	calls := 0
	executor := NewExecutor()
	executor.isolateProcessTree = true
	executor.buildCmd = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		calls++
		cmd = exec.CommandContext(ctx, "/nonexistent/playwright-isolation-no-executable")
		return cmd
	}
	_, err := executor.Run(context.Background(), &ScheduleConfig{Command: "./fixture", PlaywrightDir: t.TempDir(), Timeout: Duration{time.Second}})
	if err == nil || calls != 1 || cmd.SysProcAttr == nil || cmd.SysProcAttr.Cloneflags&syscall.CLONE_NEWPID == 0 {
		t.Fatalf("isolation failed open: calls=%d error=%v", calls, err)
	}
}
