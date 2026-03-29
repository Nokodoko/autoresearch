package runner

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// KillProcess sends SIGTERM to a process, then SIGKILL after grace period.
func KillProcess(cmd *exec.Cmd, grace time.Duration) error {
	if cmd.Process == nil {
		return nil
	}

	// Send SIGTERM to process group
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil {
		// Try direct kill
		cmd.Process.Signal(syscall.SIGTERM)
	}

	// Wait for grace period
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
		return nil
	case <-time.After(grace):
		// Force kill
		if err := cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process: %w", err)
		}
		<-done
		return nil
	}
}

// RunWithTimeout executes a command with a timeout and returns output.
func RunWithTimeout(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, name, args...)
	out, err := cmd.CombinedOutput()
	if timeoutCtx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("command timed out after %s", timeout)
	}
	return string(out), err
}
