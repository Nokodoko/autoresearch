package parallel

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/n0ko/autoresearch/internal/config"
	gitpkg "github.com/n0ko/autoresearch/internal/git"
	"github.com/n0ko/autoresearch/internal/llm"
	"github.com/n0ko/autoresearch/internal/loop"
	"github.com/n0ko/autoresearch/internal/metrics"
	"github.com/n0ko/autoresearch/internal/results"
	"github.com/n0ko/autoresearch/internal/runner"
)

// ChannelResult holds the result of a single parallel experiment.
type ChannelResult struct {
	Channel     int
	Worktree    *gitpkg.Worktree
	MetricValue float64
	Status      string // "keep", "discard", "crash"
	Description string
	Duration    time.Duration
	Commit      string
	Error       error
}

// Orchestrator manages parallel experiment execution.
type Orchestrator struct {
	cfg     *config.Config
	engine  *loop.Engine
	gitOps  *gitpkg.Ops
	resLog  *results.Log
	events  chan loop.Event
}

// NewOrchestrator creates a parallel orchestrator.
func NewOrchestrator(
	cfg *config.Config,
	engine *loop.Engine,
	gitOps *gitpkg.Ops,
	resLog *results.Log,
) *Orchestrator {
	return &Orchestrator{
		cfg:    cfg,
		engine: engine,
		gitOps: gitOps,
		resLog: resLog,
		events: make(chan loop.Event, 100),
	}
}

// Events returns the event channel for TUI updates.
func (o *Orchestrator) Events() chan loop.Event {
	return o.events
}

// Run executes the parallel optimization loop.
func (o *Orchestrator) Run(ctx context.Context) error {
	// Set up experiment branch
	if !o.gitOps.BranchExists(o.cfg.Branch) {
		if err := o.gitOps.CreateBranch(o.cfg.Branch); err != nil {
			return fmt.Errorf("creating branch: %w", err)
		}
	} else {
		if err := o.gitOps.CheckoutBranch(o.cfg.Branch); err != nil {
			return fmt.Errorf("checking out branch: %w", err)
		}
	}

	targetFile := o.cfg.TargetFile
	if targetFile == "" {
		targetFile = o.cfg.Script
	}

	bestMetric := metrics.WorstValue(o.cfg.Direction)
	parser := metrics.NewParser(o.cfg.MetricPattern, o.cfg.Direction)

	provider, err := llm.NewProvider(o.cfg)
	if err != nil {
		return fmt.Errorf("creating LLM provider: %w", err)
	}

	// Run baseline
	log.Println("Running baseline experiment...")
	expRunner := runner.New(o.cfg.RunCommand, o.cfg.Script, o.cfg.Timeout, o.cfg.WorkDir)
	baselineResult := expRunner.Run(ctx)
	if baselineResult.Crashed() {
		return fmt.Errorf("baseline crashed: %v", baselineResult.Err)
	}

	baselineMetric, err := parser.Parse(baselineResult.Output)
	if err != nil {
		return fmt.Errorf("baseline metric not found: %w", err)
	}

	bestMetric = baselineMetric
	log.Printf("Baseline %s: %.6f", o.cfg.Metric, baselineMetric)

	commit, _ := o.gitOps.CurrentCommit()
	o.resLog.Append(results.Result{
		Timestamp:   time.Now().UTC(),
		Iteration:   0,
		Commit:      commit,
		MetricName:  o.cfg.Metric,
		MetricValue: baselineMetric,
		BestMetric:  baselineMetric,
		Status:      "keep",
		DurationSecs: baselineResult.Duration.Seconds(),
		Description: "baseline",
	})

	// Main parallel loop
	iteration := 1
	maxIter := o.cfg.Iterations
	consecFailures := 0

	for {
		if ctx.Err() != nil {
			break
		}
		if maxIter > 0 && iteration > maxIter {
			break
		}
		if consecFailures >= o.cfg.MaxConsecutiveFailures {
			log.Printf("Halting: %d consecutive failures", consecFailures)
			break
		}

		log.Printf("--- Parallel iteration %d (%d channels) ---", iteration, o.cfg.Parallel)
		o.sendEvent(loop.Event{Type: "iteration_start", Iteration: iteration})

		// Dispatch parallel experiments
		channelResults := o.dispatchParallel(ctx, iteration, targetFile, bestMetric, parser, provider)

		// Process results: merge winners
		anyImprovement := false
		for _, cr := range channelResults {
			if cr.Status == "keep" && parser.IsBetterByThreshold(cr.MetricValue, bestMetric, o.cfg.Threshold) {
				// Merge this worktree
				if err := o.gitOps.MergeWorktree(cr.Worktree); err != nil {
					log.Printf("  Channel %d: merge conflict, skipping", cr.Channel)
					cr.Status = "conflict"
				} else {
					bestMetric = cr.MetricValue
					anyImprovement = true
					log.Printf("  Channel %d MERGED: %s = %.6f", cr.Channel, o.cfg.Metric, cr.MetricValue)
				}
			}

			// Log result
			o.resLog.Append(results.Result{
				Timestamp:    time.Now().UTC(),
				Iteration:    iteration,
				Channel:      cr.Channel,
				Commit:       cr.Commit,
				MetricName:   o.cfg.Metric,
				MetricValue:  cr.MetricValue,
				BestMetric:   bestMetric,
				Status:       cr.Status,
				DurationSecs: cr.Duration.Seconds(),
				Description:  cr.Description,
				Worktree:     cr.Worktree.Path,
			})

			o.sendEvent(loop.Event{
				Type: "result", Iteration: iteration, Channel: cr.Channel,
				Status: cr.Status, Metric: cr.MetricValue, BestMetric: bestMetric,
				Description: cr.Description,
			})
		}

		// Cleanup worktrees
		o.gitOps.CleanupWorktrees(o.cfg.WorktreeDir)

		if anyImprovement {
			consecFailures = 0
		} else {
			consecFailures++
		}

		iteration++
	}

	o.sendEvent(loop.Event{Type: "done", BestMetric: bestMetric})
	o.printSummary(bestMetric)
	return nil
}

// dispatchParallel runs N experiments in parallel using worktrees.
func (o *Orchestrator) dispatchParallel(
	ctx context.Context,
	iteration int,
	targetFile string,
	bestMetric float64,
	parser *metrics.Parser,
	provider llm.Provider,
) []ChannelResult {
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		results []ChannelResult
	)

	// Read current file contents for context
	fileContents := make(map[string]string)
	if content, err := gitpkg.ReadFileContent(o.cfg.WorkDir, targetFile); err == nil {
		fileContents[targetFile] = content
	}

	pastResults, _ := o.resLog.ReadAll()

	for ch := 0; ch < o.cfg.Parallel; ch++ {
		wg.Add(1)
		go func(channel int) {
			defer wg.Done()

			branchName := fmt.Sprintf("%s/exp-%d-ch%d", o.cfg.Branch, iteration, channel)

			// Create worktree
			wt, err := o.gitOps.CreateWorktree(o.cfg.WorktreeDir, branchName)
			if err != nil {
				log.Printf("  Channel %d: worktree creation failed: %v", channel, err)
				mu.Lock()
				results = append(results, ChannelResult{
					Channel: channel, Status: "crash",
					Error: err, Worktree: &gitpkg.Worktree{},
				})
				mu.Unlock()
				return
			}

			// Get proposal from LLM
			proposal, err := provider.Propose(ctx, llm.ProposalRequest{
				FileContents:    fileContents,
				PastResults:     pastResults,
				MetricName:      o.cfg.Metric,
				MetricDirection: o.cfg.Direction,
				BestMetric:      bestMetric,
			})
			if err != nil {
				log.Printf("  Channel %d: LLM proposal failed: %v", channel, err)
				mu.Lock()
				results = append(results, ChannelResult{
					Channel: channel, Status: "crash",
					Error: err, Worktree: wt, Description: "LLM proposal failed",
				})
				mu.Unlock()
				return
			}

			// Apply change in worktree
			if err := gitpkg.ApplyFileContent(wt.Path, proposal.TargetFile, proposal.NewContent); err != nil {
				log.Printf("  Channel %d: apply failed: %v", channel, err)
				mu.Lock()
				results = append(results, ChannelResult{
					Channel: channel, Status: "crash",
					Error: err, Worktree: wt, Description: proposal.Description,
				})
				mu.Unlock()
				return
			}

			// Commit in worktree
			commitMsg := fmt.Sprintf("autoresearch iter %d ch%d: %s", iteration, channel, proposal.Description)
			commit, err := o.gitOps.CommitIn(wt.Path, commitMsg)
			if err != nil {
				log.Printf("  Channel %d: commit failed: %v", channel, err)
				mu.Lock()
				results = append(results, ChannelResult{
					Channel: channel, Status: "crash",
					Error: err, Worktree: wt, Description: proposal.Description,
				})
				mu.Unlock()
				return
			}

			// Run experiment in worktree
			expRunner := runner.New(o.cfg.RunCommand, o.cfg.Script, o.cfg.Timeout, wt.Path)
			o.sendEvent(loop.Event{
				Type: "running", Iteration: iteration, Channel: channel,
				Description: proposal.Description,
			})
			expResult := expRunner.RunIn(ctx, wt.Path)

			cr := ChannelResult{
				Channel:  channel,
				Worktree: wt,
				Duration: expResult.Duration,
				Commit:   commit,
				Description: proposal.Description,
			}

			if expResult.Crashed() {
				cr.Status = "crash"
				cr.Error = expResult.Err
			} else {
				metricVal, err := parser.Parse(expResult.Output)
				if err != nil {
					cr.Status = "crash"
					cr.Error = err
				} else {
					cr.MetricValue = metricVal
					if parser.IsBetter(metricVal, bestMetric) {
						cr.Status = "keep"
					} else {
						cr.Status = "discard"
					}
				}
			}

			mu.Lock()
			results = append(results, cr)
			mu.Unlock()

			log.Printf("  Channel %d: %s (%.6f) - %s", channel, cr.Status, cr.MetricValue, proposal.Description)
		}(ch)
	}

	wg.Wait()

	// Sort by metric (best first)
	sort.Slice(results, func(i, j int) bool {
		if parser.Direction() == "maximize" {
			return results[i].MetricValue > results[j].MetricValue
		}
		return results[i].MetricValue < results[j].MetricValue
	})

	return results
}

func (o *Orchestrator) sendEvent(ev loop.Event) {
	select {
	case o.events <- ev:
	default:
	}
}

func (o *Orchestrator) printSummary(bestMetric float64) {
	allResults, _ := o.resLog.ReadAll()
	stats := results.ComputeStats(allResults)

	fmt.Println("\n--- autoresearch parallel summary ---")
	fmt.Printf("Total experiments: %d\n", stats.Total)
	fmt.Printf("Kept: %d | Discarded: %d | Crashed: %d\n", stats.Kept, stats.Discarded, stats.Crashed)
	fmt.Printf("Keep rate: %.1f%%\n", stats.KeepRate*100)
	fmt.Printf("Best %s: %.6f\n", o.cfg.Metric, bestMetric)
	if stats.Total > 1 {
		fmt.Printf("Baseline: %.6f\n", stats.BaselineMetric)
		fmt.Printf("Total improvement: %.6f (%.2f%%)\n", stats.Improvement, stats.ImprovementPct*100)
	}
}
