package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

const (
	maxReportBytes     = 64 << 20
	maxStderrBytes     = 64 << 10
	outputDrainTimeout = time.Second
)

type boundedOutput struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := max(b.limit-b.buffer.Len(), 0)
	if len(p) > remaining {
		b.truncated = true
		p = p[:remaining]
	}
	_, _ = b.buffer.Write(p)
	return n, nil
}

func (b *boundedOutput) Bytes() []byte {
	return b.buffer.Bytes()
}

func (b *boundedOutput) String() string {
	if b.truncated {
		return b.buffer.String() + " [stderr truncated]"
	}
	return b.buffer.String()
}

var errOutputDrainTimeout = errors.New("output pipes remained open after command exit")

type capturedPipe struct {
	reader, writer *os.File
	done           chan struct{}
	err            error
	output         boundedOutput
}

type commandOutput struct {
	stdout, stderr capturedPipe
}

func newCommandOutput(newPipe func() (*os.File, *os.File, error)) (*commandOutput, error) {
	output := &commandOutput{}
	var err error
	output.stdout.reader, output.stdout.writer, err = newPipe()
	if err != nil {
		return nil, fmt.Errorf("open stdout pipe: %w", err)
	}
	output.stderr.reader, output.stderr.writer, err = newPipe()
	if err != nil {
		output.close()
		return nil, fmt.Errorf("open stderr pipe: %w", err)
	}
	output.stdout.output.limit = maxReportBytes
	output.stderr.output.limit = maxStderrBytes
	return output, nil
}

func (c *commandOutput) run(cmd *exec.Cmd, drainTimeout time.Duration) (runErr, captureErr error) {
	defer c.close()
	// File writers keep exec.Cmd from owning the output copy goroutines. Its
	// Wait otherwise hides copy errors (including ErrWaitDelay) on nonzero exit.
	cmd.Stdout = c.stdout.writer
	cmd.Stderr = c.stderr.writer
	runErr = cmd.Start()
	_ = c.stdout.writer.Close()
	_ = c.stderr.writer.Close()
	if runErr != nil {
		return runErr, nil
	}
	c.stdout.start()
	c.stderr.start()
	runErr = cmd.Wait()
	return runErr, c.wait(drainTimeout)
}

func (p *capturedPipe) start() {
	p.done = make(chan struct{})
	go func() {
		_, p.err = io.Copy(&p.output, p.reader)
		_ = p.reader.Close()
		close(p.done)
	}()
}

func (c *commandOutput) wait(timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	stdoutDone, stderrDone := c.stdout.done, c.stderr.done
	var timeoutErr error
	for stdoutDone != nil || stderrDone != nil {
		select {
		case <-stdoutDone:
			stdoutDone = nil
		case <-stderrDone:
			stderrDone = nil
		case <-timer.C:
			// A completed drain must not fail just because the timer is also ready.
			select {
			case <-c.stdout.done:
				stdoutDone = nil
			default:
			}
			select {
			case <-c.stderr.done:
				stderrDone = nil
			default:
			}
			if stdoutDone == nil && stderrDone == nil {
				break
			}
			timeoutErr = fmt.Errorf("%w (drain limit %s)", errOutputDrainTimeout, timeout)
			// Close only our readers, then join both copies before inspecting
			// their errors or buffers. Descendant processes are not terminated.
			_ = c.stdout.reader.Close()
			_ = c.stderr.reader.Close()
			<-c.stdout.done
			<-c.stderr.done
			stdoutDone, stderrDone = nil, nil
		}
	}
	return errors.Join(timeoutErr, capturePipeError("stdout", c.stdout.err), capturePipeError("stderr", c.stderr.err))
}

func capturePipeError(stream string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s capture: %w", stream, err)
}

func (c *commandOutput) close() {
	for _, file := range []*os.File{c.stdout.reader, c.stdout.writer, c.stderr.reader, c.stderr.writer} {
		if file != nil {
			_ = file.Close()
		}
	}
}
