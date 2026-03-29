package guardrails

import (
	"testing"

	"github.com/n0ko/autoresearch/internal/config"
)

func TestShouldContinue(t *testing.T) {
	cfg := &config.Config{
		Iterations:             5,
		MaxConsecutiveFailures: 3,
		MaxTotalCrashes:        10,
	}
	g := New(cfg)

	// Should continue initially
	if err := g.ShouldContinue(); err != nil {
		t.Errorf("should continue initially: %v", err)
	}

	// Run 5 successful iterations
	for i := 0; i < 5; i++ {
		g.RecordSuccess()
	}

	// Should stop after max iterations
	if err := g.ShouldContinue(); err == nil {
		t.Error("should stop after max iterations")
	}
}

func TestConsecutiveFailures(t *testing.T) {
	cfg := &config.Config{
		MaxConsecutiveFailures: 3,
		MaxTotalCrashes:        10,
	}
	g := New(cfg)

	// Record 3 failures
	g.RecordFailure()
	g.RecordFailure()
	g.RecordFailure()

	if err := g.ShouldContinue(); err == nil {
		t.Error("should stop after consecutive failures")
	}
}

func TestConsecutiveFailuresReset(t *testing.T) {
	cfg := &config.Config{
		MaxConsecutiveFailures: 3,
		MaxTotalCrashes:        10,
	}
	g := New(cfg)

	g.RecordFailure()
	g.RecordFailure()
	g.RecordSuccess() // Resets consecutive failures

	if err := g.ShouldContinue(); err != nil {
		t.Errorf("should continue after success resets consecutive failures: %v", err)
	}
}

func TestShutdown(t *testing.T) {
	cfg := &config.Config{}
	g := New(cfg)

	if g.IsShutdownRequested() {
		t.Error("shutdown should not be requested initially")
	}

	g.RequestShutdown()

	if !g.IsShutdownRequested() {
		t.Error("shutdown should be requested after RequestShutdown")
	}

	if err := g.ShouldContinue(); err == nil {
		t.Error("should not continue after shutdown requested")
	}
}

func TestUnlimitedIterations(t *testing.T) {
	cfg := &config.Config{
		Iterations:             0, // unlimited
		MaxConsecutiveFailures: 100,
		MaxTotalCrashes:        100,
	}
	g := New(cfg)

	for i := 0; i < 50; i++ {
		g.RecordSuccess()
	}

	if err := g.ShouldContinue(); err != nil {
		t.Errorf("unlimited iterations should continue: %v", err)
	}
}
