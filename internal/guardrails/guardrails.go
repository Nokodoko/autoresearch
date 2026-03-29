package guardrails

import (
	"fmt"
	"sync/atomic"

	"github.com/n0ko/autoresearch/internal/config"
)

// Guard implements safety rails for the experiment loop.
type Guard struct {
	maxIterations       int
	maxConsecFailures   int
	maxTotalCrashes     int
	threshold           float64

	consecFailures      int
	totalCrashes        int
	totalIterations     int
	shutdownRequested   atomic.Bool
}

// New creates a Guard from configuration.
func New(cfg *config.Config) *Guard {
	return &Guard{
		maxIterations:     cfg.Iterations,
		maxConsecFailures: cfg.MaxConsecutiveFailures,
		maxTotalCrashes:   cfg.MaxTotalCrashes,
		threshold:         cfg.Threshold,
	}
}

// ShouldContinue returns true if the loop should continue, or an error explaining why it should stop.
func (g *Guard) ShouldContinue() error {
	if g.shutdownRequested.Load() {
		return fmt.Errorf("shutdown requested")
	}
	if g.maxIterations > 0 && g.totalIterations >= g.maxIterations {
		return fmt.Errorf("max iterations reached (%d)", g.maxIterations)
	}
	if g.maxConsecFailures > 0 && g.consecFailures >= g.maxConsecFailures {
		return fmt.Errorf("max consecutive failures reached (%d)", g.maxConsecFailures)
	}
	if g.maxTotalCrashes > 0 && g.totalCrashes >= g.maxTotalCrashes {
		return fmt.Errorf("max total crashes reached (%d)", g.maxTotalCrashes)
	}
	return nil
}

// RecordSuccess records a successful experiment (keep or discard).
func (g *Guard) RecordSuccess() {
	g.totalIterations++
	g.consecFailures = 0
}

// RecordFailure records a failed experiment (crash).
func (g *Guard) RecordFailure() {
	g.totalIterations++
	g.consecFailures++
	g.totalCrashes++
}

// RequestShutdown signals the loop to stop after the current iteration.
func (g *Guard) RequestShutdown() {
	g.shutdownRequested.Store(true)
}

// IsShutdownRequested returns true if shutdown has been requested.
func (g *Guard) IsShutdownRequested() bool {
	return g.shutdownRequested.Load()
}

// Threshold returns the minimum improvement threshold.
func (g *Guard) Threshold() float64 {
	return g.threshold
}

// Iteration returns the current iteration count.
func (g *Guard) Iteration() int {
	return g.totalIterations
}

// MaxIterations returns the max iteration limit (0 = unlimited).
func (g *Guard) MaxIterations() int {
	return g.maxIterations
}

// Stats returns guard rail statistics.
func (g *Guard) Stats() GuardStats {
	return GuardStats{
		TotalIterations:    g.totalIterations,
		MaxIterations:      g.maxIterations,
		ConsecFailures:     g.consecFailures,
		TotalCrashes:       g.totalCrashes,
		ShutdownRequested:  g.shutdownRequested.Load(),
	}
}

// GuardStats holds guard rail statistics for display.
type GuardStats struct {
	TotalIterations   int
	MaxIterations     int
	ConsecFailures    int
	TotalCrashes      int
	ShutdownRequested bool
}
