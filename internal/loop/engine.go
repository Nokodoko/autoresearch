package loop

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/n0ko/autoresearch/internal/config"
	gitpkg "github.com/n0ko/autoresearch/internal/git"
	"github.com/n0ko/autoresearch/internal/guardrails"
	"github.com/n0ko/autoresearch/internal/llm"
	"github.com/n0ko/autoresearch/internal/metrics"
	"github.com/n0ko/autoresearch/internal/ob1"
	"github.com/n0ko/autoresearch/internal/results"
	"github.com/n0ko/autoresearch/internal/runner"
)

// Event represents a loop event for TUI updates.
type Event struct {
	Type       string  // "iteration_start", "proposal", "running", "result", "done", "error"
	Iteration  int
	Channel    int
	Status     string  // "keep", "discard", "crash"
	Metric     float64
	BestMetric float64
	Description string
	Error      string
}

// Engine runs the core optimization loop.
type Engine struct {
	cfg         *config.Config
	gitOps      *gitpkg.Ops
	resLog      *results.Log
	parser      *metrics.Parser
	runner      *runner.Runner
	provider    llm.Provider
	guard       *guardrails.Guard
	ob1Client   *ob1.Client // nil if ob1 integration disabled
	bestMetric  float64
	events      chan Event
}

// NewEngine creates a new loop engine.
// ob1Client may be nil to disable OpenBrain integration.
func NewEngine(
	cfg *config.Config,
	gitOps *gitpkg.Ops,
	resLog *results.Log,
	parser *metrics.Parser,
	expRunner *runner.Runner,
	provider llm.Provider,
	guard *guardrails.Guard,
	ob1Client *ob1.Client,
) *Engine {
	return &Engine{
		cfg:        cfg,
		gitOps:     gitOps,
		resLog:     resLog,
		parser:     parser,
		runner:     expRunner,
		provider:   provider,
		guard:      guard,
		ob1Client:  ob1Client,
		bestMetric: metrics.WorstValue(cfg.Direction),
		events:     make(chan Event, 100),
	}
}

// Events returns the event channel for TUI updates.
func (e *Engine) Events() chan Event {
	return e.events
}

// Run executes the optimization loop until halted.
func (e *Engine) Run(ctx context.Context) error {
	// Set up experiment branch
	if err := e.setupBranch(); err != nil {
		return fmt.Errorf("setting up branch: %w", err)
	}

	// Determine target file
	targetFile := e.cfg.TargetFile
	if targetFile == "" {
		targetFile = e.cfg.Script
	}

	log.Printf("Starting autoresearch loop: metric=%s, direction=%s, backend=%s",
		e.cfg.Metric, e.cfg.Direction, e.cfg.LLMBackend)

	// Run baseline first (iteration 0)
	if err := e.runBaseline(ctx, targetFile); err != nil {
		return fmt.Errorf("baseline run: %w", err)
	}

	// Main loop
	for {
		if err := e.guard.ShouldContinue(); err != nil {
			log.Printf("Halting: %s", err)
			e.sendEvent(Event{Type: "done", BestMetric: e.bestMetric})
			break
		}

		if ctx.Err() != nil {
			log.Println("Context cancelled, halting loop")
			e.sendEvent(Event{Type: "done", BestMetric: e.bestMetric})
			break
		}

		iteration := e.guard.Iteration()
		e.sendEvent(Event{Type: "iteration_start", Iteration: iteration})

		if err := e.runIteration(ctx, targetFile, iteration); err != nil {
			log.Printf("Iteration %d error: %v", iteration, err)
			e.guard.RecordFailure()
			e.sendEvent(Event{Type: "error", Iteration: iteration, Error: err.Error()})
			continue
		}
	}

	e.printSummary()
	return nil
}

// runBaseline runs the script as-is to establish the baseline metric.
func (e *Engine) runBaseline(ctx context.Context, targetFile string) error {
	log.Println("Running baseline experiment...")
	e.sendEvent(Event{Type: "running", Iteration: 0, Description: "baseline"})

	result := e.runner.Run(ctx)
	if result.Crashed() {
		return fmt.Errorf("baseline crashed: %v\nOutput: %s", result.Err, truncate(result.Output, 500))
	}

	metricVal, err := e.parser.Parse(result.Output)
	if err != nil {
		return fmt.Errorf("baseline metric not found: %w\nOutput: %s", err, truncate(result.Output, 500))
	}

	e.bestMetric = metricVal
	log.Printf("Baseline %s: %.6f", e.cfg.Metric, metricVal)

	commit, _ := e.gitOps.CurrentCommit()
	r := results.Result{
		Timestamp:    time.Now().UTC(),
		Iteration:    0,
		Commit:       commit,
		MetricName:   e.cfg.Metric,
		MetricValue:  metricVal,
		BestMetric:   metricVal,
		Status:       "keep",
		DurationSecs: result.Duration.Seconds(),
		Description:  "baseline",
	}
	e.resLog.Append(r)
	e.writeToOB1(ctx, r)
	e.guard.RecordSuccess()

	e.sendEvent(Event{
		Type: "result", Iteration: 0, Status: "keep",
		Metric: metricVal, BestMetric: metricVal, Description: "baseline",
	})

	return nil
}

// runIteration runs a single optimization iteration.
func (e *Engine) runIteration(ctx context.Context, targetFile string, iteration int) error {
	// 1. Build context
	loopCtx, err := BuildContext(e.gitOps, e.resLog, e.ob1Client, targetFile, e.cfg.WorkDir, iteration, e.bestMetric, e.cfg.Metric)
	if err != nil {
		return fmt.Errorf("building context: %w", err)
	}

	// 2. Get LLM proposal
	e.sendEvent(Event{Type: "proposal", Iteration: iteration})

	proposal, err := e.provider.Propose(ctx, llm.ProposalRequest{
		FileContents:    loopCtx.FileContents,
		PastResults:     loopCtx.PastResults,
		MetricName:      e.cfg.Metric,
		MetricDirection: e.cfg.Direction,
		BestMetric:      e.bestMetric,
		OB1History:      loopCtx.OB1History,
	})
	if err != nil {
		return fmt.Errorf("LLM proposal: %w", err)
	}

	log.Printf("Iteration %d: %s", iteration, proposal.Description)

	// 3. Apply the change
	if err := gitpkg.ApplyFileContent(e.cfg.WorkDir, proposal.TargetFile, proposal.NewContent); err != nil {
		return fmt.Errorf("applying change: %w", err)
	}

	// Commit the change before running
	commitMsg := fmt.Sprintf("autoresearch iter %d: %s", iteration, proposal.Description)
	commit, err := e.gitOps.Commit(commitMsg)
	if err != nil {
		e.gitOps.ResetHard()
		return fmt.Errorf("committing change: %w", err)
	}

	// 4. Run experiment
	e.sendEvent(Event{Type: "running", Iteration: iteration, Description: proposal.Description})

	expResult := e.runner.Run(ctx)

	// 5. Parse metric
	if expResult.Crashed() {
		log.Printf("Iteration %d CRASHED: %v", iteration, expResult.Err)
		e.gitOps.RevertLastCommit()

		r := results.Result{
			Timestamp:    time.Now().UTC(),
			Iteration:    iteration,
			Commit:       commit,
			MetricName:   e.cfg.Metric,
			MetricValue:  0,
			BestMetric:   e.bestMetric,
			Status:       "crash",
			DurationSecs: expResult.Duration.Seconds(),
			Description:  proposal.Description,
			Error:        truncate(fmt.Sprintf("%v", expResult.Err), 200),
		}
		e.resLog.Append(r)
		e.writeToOB1(ctx, r)
		e.guard.RecordFailure()

		e.sendEvent(Event{
			Type: "result", Iteration: iteration, Status: "crash",
			BestMetric: e.bestMetric, Description: proposal.Description,
		})
		return nil
	}

	metricVal, err := e.parser.Parse(expResult.Output)
	if err != nil {
		log.Printf("Iteration %d: metric not found, treating as crash", iteration)
		e.gitOps.RevertLastCommit()

		r := results.Result{
			Timestamp:    time.Now().UTC(),
			Iteration:    iteration,
			Commit:       commit,
			MetricName:   e.cfg.Metric,
			MetricValue:  0,
			BestMetric:   e.bestMetric,
			Status:       "crash",
			DurationSecs: expResult.Duration.Seconds(),
			Description:  proposal.Description,
			Error:        err.Error(),
		}
		e.resLog.Append(r)
		e.writeToOB1(ctx, r)
		e.guard.RecordFailure()

		e.sendEvent(Event{
			Type: "result", Iteration: iteration, Status: "crash",
			BestMetric: e.bestMetric, Description: proposal.Description,
		})
		return nil
	}

	// 6. Decide: keep or discard
	improved := e.parser.IsBetterByThreshold(metricVal, e.bestMetric, e.guard.Threshold())
	status := "discard"

	if improved {
		status = "keep"
		e.bestMetric = metricVal
		log.Printf("Iteration %d KEEP: %s = %.6f (improved)", iteration, e.cfg.Metric, metricVal)
	} else {
		e.gitOps.RevertLastCommit()
		log.Printf("Iteration %d DISCARD: %s = %.6f (no improvement over %.6f)",
			iteration, e.cfg.Metric, metricVal, e.bestMetric)
	}

	r := results.Result{
		Timestamp:    time.Now().UTC(),
		Iteration:    iteration,
		Commit:       commit,
		MetricName:   e.cfg.Metric,
		MetricValue:  metricVal,
		BestMetric:   e.bestMetric,
		Status:       status,
		DurationSecs: expResult.Duration.Seconds(),
		Description:  proposal.Description,
	}
	e.resLog.Append(r)
	e.writeToOB1(ctx, r)
	e.guard.RecordSuccess()

	e.sendEvent(Event{
		Type: "result", Iteration: iteration, Status: status,
		Metric: metricVal, BestMetric: e.bestMetric, Description: proposal.Description,
	})

	return nil
}

func (e *Engine) setupBranch() error {
	if e.gitOps.BranchExists(e.cfg.Branch) {
		return e.gitOps.CheckoutBranch(e.cfg.Branch)
	}
	return e.gitOps.CreateBranch(e.cfg.Branch)
}

func (e *Engine) sendEvent(ev Event) {
	select {
	case e.events <- ev:
	default:
		// Don't block if nobody is listening
	}
}

func (e *Engine) printSummary() {
	allResults, _ := e.resLog.ReadAll()
	stats := results.ComputeStats(allResults)

	fmt.Println("\n--- autoresearch summary ---")
	fmt.Printf("Total iterations: %d\n", stats.Total)
	fmt.Printf("Kept: %d | Discarded: %d | Crashed: %d\n", stats.Kept, stats.Discarded, stats.Crashed)
	fmt.Printf("Keep rate: %.1f%%\n", stats.KeepRate*100)
	fmt.Printf("Best %s: %.6f\n", e.cfg.Metric, e.bestMetric)
	if stats.Total > 1 {
		fmt.Printf("Baseline: %.6f\n", stats.BaselineMetric)
		fmt.Printf("Total improvement: %.6f (%.2f%%)\n", stats.Improvement, stats.ImprovementPct*100)
	}
}

// writeToOB1 writes an experiment result to OpenBrain (best-effort).
// If the ob1 client is nil or the write fails, it logs a warning and continues.
func (e *Engine) writeToOB1(ctx context.Context, r results.Result) {
	if e.ob1Client == nil {
		return
	}
	entry := ob1.ExperimentEntry{
		Iteration:   r.Iteration,
		Channel:     r.Channel,
		MetricName:  r.MetricName,
		MetricValue: r.MetricValue,
		BestMetric:  r.BestMetric,
		Status:      r.Status,
		Description: r.Description,
		Commit:      r.Commit,
		Branch:      e.cfg.Branch,
		Timestamp:   r.Timestamp,
	}
	if err := e.ob1Client.WriteExperimentResult(ctx, entry); err != nil {
		log.Printf("ob1: failed to write result: %v", err)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
