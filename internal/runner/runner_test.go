package runner

import (
	"context"
	"testing"
	"time"
)

func TestRunSimpleScript(t *testing.T) {
	r := New("", "echo", 10*time.Second, ".")
	// Override to just run echo
	ctx := context.Background()
	result := r.RunIn(ctx, ".")

	// echo runs as a plain command since it has no extension
	// This tests the basic execution path
	if result.Duration <= 0 {
		t.Error("duration should be > 0")
	}
}

func TestCrashedDetection(t *testing.T) {
	r := ExperimentResult{ExitCode: 1}
	if !r.Crashed() {
		t.Error("exit code 1 should be detected as crash")
	}

	r = ExperimentResult{ExitCode: 0}
	if r.Crashed() {
		t.Error("exit code 0 should not be a crash")
	}
}

func TestString(t *testing.T) {
	r := ExperimentResult{ExitCode: 0, Duration: 5 * time.Second}
	s := r.String()
	if s == "" {
		t.Error("String() should not be empty")
	}

	r = ExperimentResult{ExitCode: 1, Duration: 3 * time.Second}
	s = r.String()
	if s == "" {
		t.Error("String() should not be empty for crash")
	}
}
