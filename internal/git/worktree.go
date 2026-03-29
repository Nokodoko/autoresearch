package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Worktree represents a git worktree.
type Worktree struct {
	Path   string
	Branch string
}

// CreateWorktree creates a new git worktree at the given path with a new branch.
func (g *Ops) CreateWorktree(worktreeDir, branchName string) (*Worktree, error) {
	wtPath := filepath.Join(worktreeDir, branchName)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(wtPath), 0o755); err != nil {
		return nil, fmt.Errorf("creating worktree parent dir: %w", err)
	}

	_, err := g.run("worktree", "add", wtPath, "-b", branchName)
	if err != nil {
		return nil, fmt.Errorf("creating worktree: %w", err)
	}

	return &Worktree{Path: wtPath, Branch: branchName}, nil
}

// RemoveWorktree removes a git worktree and its branch.
func (g *Ops) RemoveWorktree(wt *Worktree) error {
	// Force remove worktree
	if _, err := g.run("worktree", "remove", wt.Path, "--force"); err != nil {
		// If worktree dir doesn't exist, try to prune
		g.run("worktree", "prune")
	}

	// Delete the branch
	g.run("branch", "-D", wt.Branch)
	return nil
}

// ListWorktrees returns all active worktrees.
func (g *Ops) ListWorktrees() ([]Worktree, error) {
	out, err := g.run("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	var worktrees []Worktree
	var current Worktree
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "worktree ") {
			current = Worktree{Path: strings.TrimPrefix(line, "worktree ")}
		} else if strings.HasPrefix(line, "branch ") {
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		} else if line == "" && current.Path != "" {
			worktrees = append(worktrees, current)
			current = Worktree{}
		}
	}
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	return worktrees, nil
}

// MergeWorktree merges a worktree branch into the current branch.
func (g *Ops) MergeWorktree(wt *Worktree) error {
	_, err := g.run("merge", wt.Branch, "--no-edit")
	return err
}

// MergeWorktreeNoFF merges with no fast-forward to preserve history.
func (g *Ops) MergeWorktreeNoFF(wt *Worktree, message string) error {
	_, err := g.run("merge", wt.Branch, "--no-ff", "-m", message)
	return err
}

// CleanupWorktrees removes all worktrees in the given directory.
func (g *Ops) CleanupWorktrees(worktreeDir string) error {
	entries, err := os.ReadDir(worktreeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			wtPath := filepath.Join(worktreeDir, entry.Name())
			g.run("worktree", "remove", wtPath, "--force")
			g.run("branch", "-D", entry.Name())
		}
	}

	g.run("worktree", "prune")
	return nil
}
