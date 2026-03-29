package loop

import (
	"fmt"

	gitpkg "github.com/n0ko/autoresearch/internal/git"
	"github.com/n0ko/autoresearch/internal/results"
)

// Context holds the current state for an iteration.
type Context struct {
	FileContents map[string]string
	GitLog       string
	PastResults  []results.Result
	BestMetric   float64
	Iteration    int
}

// BuildContext gathers all context needed for an LLM proposal.
func BuildContext(gitOps *gitpkg.Ops, resLog *results.Log, targetFile, workDir string, iteration int, bestMetric float64) (*Context, error) {
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

	return &Context{
		FileContents: files,
		GitLog:       gitLog,
		PastResults:  pastResults,
		BestMetric:   bestMetric,
		Iteration:    iteration,
	}, nil
}
