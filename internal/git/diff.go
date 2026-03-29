package git

import (
	"fmt"
	"os"
	"path/filepath"
)

// ApplyFileContent writes new content to a file, creating it if necessary.
// This is the primary way to apply LLM-proposed changes -- the LLM sends
// complete file content rather than unified diffs for reliability.
func ApplyFileContent(dir, filename, content string) error {
	path := filepath.Join(dir, filename)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing file %s: %w", path, err)
	}
	return nil
}

// ReadFileContent reads the content of a file.
func ReadFileContent(dir, filename string) (string, error) {
	path := filepath.Join(dir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading file %s: %w", path, err)
	}
	return string(data), nil
}

// ApplyPatch applies a unified diff patch using git apply.
func (g *Ops) ApplyPatch(dir, patch string) error {
	// Write patch to temp file
	tmpFile := filepath.Join(dir, ".autoresearch-patch.tmp")
	if err := os.WriteFile(tmpFile, []byte(patch), 0o644); err != nil {
		return fmt.Errorf("writing patch file: %w", err)
	}
	defer os.Remove(tmpFile)

	_, err := g.runIn(dir, "apply", "--stat", tmpFile)
	if err != nil {
		return fmt.Errorf("applying patch: %w", err)
	}

	_, err = g.runIn(dir, "apply", tmpFile)
	return err
}
