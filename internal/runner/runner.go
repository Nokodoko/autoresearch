package runner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ExperimentResult holds the output of an experiment run.
type ExperimentResult struct {
	Output   string
	ExitCode int
	Duration time.Duration
	Err      error
}

// Runner executes experiment scripts.
type Runner struct {
	runCommand string
	script     string
	timeout    time.Duration
	workDir    string
}

// New creates a new Runner.
func New(runCommand, script string, timeout time.Duration, workDir string) *Runner {
	return &Runner{
		runCommand: runCommand,
		script:     script,
		timeout:    timeout,
		workDir:    workDir,
	}
}

// Run executes the experiment script and captures output.
func (r *Runner) Run(ctx context.Context) ExperimentResult {
	return r.RunIn(ctx, r.workDir)
}

// RunIn executes the experiment script in a specific directory.
func (r *Runner) RunIn(ctx context.Context, dir string) ExperimentResult {
	start := time.Now()

	// Build command
	var cmd *exec.Cmd
	timeoutCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	if r.runCommand != "" {
		// Split run command, e.g., "uv run" -> ["uv", "run"]
		parts := strings.Fields(r.runCommand)
		args := append(parts[1:], r.script)
		cmd = exec.CommandContext(timeoutCtx, parts[0], args...)
	} else {
		// Auto-detect: try common runners
		cmd = r.buildAutoCommand(timeoutCtx)
	}

	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	// Combine stdout and stderr
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return ExperimentResult{
		Output:   output,
		ExitCode: exitCode,
		Duration: duration,
		Err:      err,
	}
}

// buildAutoCommand tries to detect the right command to run the script.
func (r *Runner) buildAutoCommand(ctx context.Context) *exec.Cmd {
	ext := ""
	if idx := strings.LastIndex(r.script, "."); idx >= 0 {
		ext = r.script[idx:]
	}

	switch ext {
	case ".py":
		// Try uv first, then python
		if _, err := exec.LookPath("uv"); err == nil {
			return exec.CommandContext(ctx, "uv", "run", r.script)
		}
		return exec.CommandContext(ctx, "python", r.script)
	case ".sh":
		return exec.CommandContext(ctx, "bash", r.script)
	case ".go":
		return exec.CommandContext(ctx, "go", "run", r.script)
	default:
		return exec.CommandContext(ctx, r.script)
	}
}

// Crashed returns true if the experiment crashed (non-zero exit).
func (r ExperimentResult) Crashed() bool {
	return r.ExitCode != 0 || r.Err != nil
}

// String returns a human-readable summary of the result.
func (r ExperimentResult) String() string {
	if r.Crashed() {
		return fmt.Sprintf("CRASHED (exit=%d, duration=%s, err=%v)", r.ExitCode, r.Duration.Round(time.Second), r.Err)
	}
	return fmt.Sprintf("OK (duration=%s)", r.Duration.Round(time.Second))
}
