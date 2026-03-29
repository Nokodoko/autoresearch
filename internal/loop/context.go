package loop

import (
	"context"
	"fmt"
	"log"

	gitpkg "github.com/n0ko/autoresearch/internal/git"
	"github.com/n0ko/autoresearch/internal/ob1"
	"github.com/n0ko/autoresearch/internal/results"
)

// Context holds the current state for an iteration.
type Context struct {
	FileContents map[string]string
	GitLog       string
	PastResults  []results.Result
	BestMetric   float64
	Iteration    int
	OB1History   string // Formatted ob1 experiment history (empty if ob1 disabled)
}

// BuildContext gathers all context needed for an LLM proposal.
// If ob1Client is non-nil, experiment history is fetched from OpenBrain.
func BuildContext(gitOps *gitpkg.Ops, resLog *results.Log, ob1Client *ob1.Client, targetFile, workDir string, iteration int, bestMetric float64, metricName string) (*Context, error) {
	// Read target file
	content, err := gitpkg.ReadFileContent(workDir, targetFile)
	if err != nil {
		return nil, fmt.Errorf("reading target file %s: %w", targetFile, err)
	}

	files := map[string]string{
		targetFile: content,
	}

	// Read git log
	gitLog, err := gitOps.Log(20)
	if err != nil {
		gitLog = "(no git log available)"
	}

	// Read past results
	pastResults, err := resLog.ReadAll()
	if err != nil {
		pastResults = nil
	}

	// Fetch ob1 experiment history (best-effort).
	var ob1History string
	if ob1Client != nil {
		entries, err := ob1Client.ReadExperimentHistory(context.Background(), 50)
		if err != nil {
			log.Printf("ob1: failed to read experiment history: %v", err)
		} else {
			ob1History = ob1.FormatHistory(entries, metricName)
		}
	}

	return &Context{
		FileContents: files,
		GitLog:       gitLog,
		PastResults:  pastResults,
		BestMetric:   bestMetric,
		Iteration:    iteration,
		OB1History:   ob1History,
	}, nil
}
