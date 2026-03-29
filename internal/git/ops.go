package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Ops provides git operations.
type Ops struct {
	workDir string
}

// NewOps creates a new Ops for the given working directory.
func NewOps(workDir string) *Ops {
	return &Ops{workDir: workDir}
}

// run executes a git command and returns stdout.
func (g *Ops) run(args ...string) (string, error) {
	return g.runIn(g.workDir, args...)
}

// runIn executes a git command in a specific directory.
func (g *Ops) runIn(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w\nstderr: %s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

// CurrentBranch returns the current branch name.
func (g *Ops) CurrentBranch() (string, error) {
	return g.run("rev-parse", "--abbrev-ref", "HEAD")
}

// CurrentCommit returns the short hash of HEAD.
func (g *Ops) CurrentCommit() (string, error) {
	return g.run("rev-parse", "--short=7", "HEAD")
}

// CreateBranch creates and checks out a new branch from current HEAD.
func (g *Ops) CreateBranch(name string) error {
	_, err := g.run("checkout", "-b", name)
	return err
}

// CheckoutBranch checks out an existing branch.
func (g *Ops) CheckoutBranch(name string) error {
	_, err := g.run("checkout", name)
	return err
}

// BranchExists checks if a branch exists.
func (g *Ops) BranchExists(name string) bool {
	_, err := g.run("rev-parse", "--verify", name)
	return err == nil
}

// Commit stages all changes and commits with the given message.
func (g *Ops) Commit(message string) (string, error) {
	if _, err := g.run("add", "-A"); err != nil {
		return "", err
	}
	if _, err := g.run("commit", "-m", message); err != nil {
		return "", err
	}
	return g.CurrentCommit()
}

// CommitIn stages all changes in a directory and commits.
func (g *Ops) CommitIn(dir, message string) (string, error) {
	if _, err := g.runIn(dir, "add", "-A"); err != nil {
		return "", err
	}
	if _, err := g.runIn(dir, "commit", "-m", message); err != nil {
		return "", err
	}
	return g.runIn(dir, "rev-parse", "--short=7", "HEAD")
}

// ResetHard resets the working directory to HEAD (discards all changes).
func (g *Ops) ResetHard() error {
	_, err := g.run("reset", "--hard", "HEAD")
	return err
}

// ResetHardTo resets to a specific commit.
func (g *Ops) ResetHardTo(ref string) error {
	_, err := g.run("reset", "--hard", ref)
	return err
}

// RevertLastCommit undoes the last commit, keeping working tree clean.
func (g *Ops) RevertLastCommit() error {
	_, err := g.run("reset", "--hard", "HEAD~1")
	return err
}

// Log returns the last N commit messages.
func (g *Ops) Log(n int) (string, error) {
	return g.run("log", fmt.Sprintf("-%d", n), "--oneline")
}

// LogDetailed returns detailed log with diffs for the last N commits.
func (g *Ops) LogDetailed(n int) (string, error) {
	return g.run("log", fmt.Sprintf("-%d", n), "--stat", "--format=format:%h %s")
}

// Diff returns the diff of staged and unstaged changes.
func (g *Ops) Diff() (string, error) {
	return g.run("diff")
}

// HasChanges returns true if there are uncommitted changes.
func (g *Ops) HasChanges() (bool, error) {
	out, err := g.run("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// StashPush stashes current changes.
func (g *Ops) StashPush() error {
	_, err := g.run("stash", "push", "-m", "autoresearch-stash")
	return err
}

// StashPop pops the latest stash.
func (g *Ops) StashPop() error {
	_, err := g.run("stash", "pop")
	return err
}
