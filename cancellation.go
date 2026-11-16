package main

import (
	"context"
	"os"
	"os/exec"
	"time"
)

const cancellationGracePeriod = 5 * time.Second

func newRunnerCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	// #nosec G702 -- Operator-configured executable and direct arguments are the runner contract; no shell is introduced.
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Cancel = func() error {
		return cmd.Process.Signal(os.Interrupt)
	}
	cmd.WaitDelay = cancellationGracePeriod
	return cmd
}
