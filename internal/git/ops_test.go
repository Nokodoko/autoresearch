package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func setupTestRepo(t *testing.T) (string, *Ops) {
	t.Helper()
	dir := t.TempDir()

	// Initialize git repo
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git setup: %v\n%s", err, out)
		}
	}

	// Create initial commit
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0o644)
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = dir
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = dir
	cmd.Run()

	return dir, NewOps(dir)
}

func TestCurrentBranch(t *testing.T) {
	_, ops := setupTestRepo(t)

	branch, err := ops.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	// Could be "main" or "master" depending on git config
	if branch != "main" && branch != "master" {
		t.Errorf("unexpected branch: %s", branch)
	}
}

func TestCreateAndCheckoutBranch(t *testing.T) {
	_, ops := setupTestRepo(t)

	err := ops.CreateBranch("test-branch")
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	branch, err := ops.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != "test-branch" {
		t.Errorf("expected test-branch, got %s", branch)
	}
}

func TestBranchExists(t *testing.T) {
	_, ops := setupTestRepo(t)

	if !ops.BranchExists("HEAD") {
		t.Error("HEAD should exist")
	}
	if ops.BranchExists("nonexistent-branch-xyz") {
		t.Error("nonexistent branch should not exist")
	}
}

func TestCommitAndRevert(t *testing.T) {
	dir, ops := setupTestRepo(t)

	// Make a change
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("changed"), 0o644)

	hash, err := ops.Commit("test commit")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if hash == "" {
		t.Error("expected non-empty commit hash")
	}

	// Verify change
	data, _ := os.ReadFile(filepath.Join(dir, "test.txt"))
	if string(data) != "changed" {
		t.Error("file should contain 'changed'")
	}

	// Revert
	err = ops.RevertLastCommit()
	if err != nil {
		t.Fatalf("RevertLastCommit: %v", err)
	}

	data, _ = os.ReadFile(filepath.Join(dir, "test.txt"))
	if string(data) != "hello" {
		t.Errorf("file should contain 'hello' after revert, got %s", string(data))
	}
}

func TestHasChanges(t *testing.T) {
	dir, ops := setupTestRepo(t)

	has, err := ops.HasChanges()
	if err != nil {
		t.Fatalf("HasChanges: %v", err)
	}
	if has {
		t.Error("should not have changes initially")
	}

	// Make a change
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new file"), 0o644)

	has, err = ops.HasChanges()
	if err != nil {
		t.Fatalf("HasChanges: %v", err)
	}
	if !has {
		t.Error("should have changes after writing file")
	}
}

func TestApplyFileContent(t *testing.T) {
	dir := t.TempDir()

	err := ApplyFileContent(dir, "subdir/test.py", "print('hello')\n")
	if err != nil {
		t.Fatalf("ApplyFileContent: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "subdir", "test.py"))
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(data) != "print('hello')\n" {
		t.Errorf("unexpected content: %s", string(data))
	}
}

func TestReadFileContent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("content here"), 0o644)

	content, err := ReadFileContent(dir, "test.txt")
	if err != nil {
		t.Fatalf("ReadFileContent: %v", err)
	}
	if content != "content here" {
		t.Errorf("unexpected content: %s", content)
	}
}
