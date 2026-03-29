package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// LlamaCppProvider implements the Provider interface for local llama.cpp models.
type LlamaCppProvider struct {
	binaryPath  string
	modelPath   string
	temperature float64
}

// NewLlamaCppProvider creates a local llama.cpp LLM provider.
func NewLlamaCppProvider(modelPath string, temperature float64) (*LlamaCppProvider, error) {
	if modelPath == "" {
		return nil, fmt.Errorf("--llm-model is required for llamacpp backend (path to .gguf model)")
	}

	// Find llama-cli binary
	binaryPath := os.Getenv("LLAMACPP_PATH")
	if binaryPath == "" {
		// Try common locations
		for _, name := range []string{"llama-cli", "llama.cpp/main", "main"} {
			if p, err := exec.LookPath(name); err == nil {
				binaryPath = p
				break
			}
		}
	}
	if binaryPath == "" {
		return nil, fmt.Errorf("llama-cli not found; set LLAMACPP_PATH or ensure llama-cli is in PATH")
	}

	return &LlamaCppProvider{
		binaryPath:  binaryPath,
		modelPath:   modelPath,
		temperature: temperature,
	}, nil
}

func (l *LlamaCppProvider) Name() string {
	return "llamacpp"
}

// Propose runs llama.cpp locally to generate a proposal.
func (l *LlamaCppProvider) Propose(ctx context.Context, req ProposalRequest) (*Proposal, error) {
	systemPrompt := BuildSystemPrompt(req.MetricName, req.MetricDirection)
	userPrompt := BuildUserPrompt(req.FileContents, req.PastResults, req.BestMetric, req.MetricName)

	fullPrompt := fmt.Sprintf("[INST] <<SYS>>\n%s\n<</SYS>>\n\n%s [/INST]", systemPrompt, userPrompt)

	args := []string{
		"-m", l.modelPath,
		"--temp", fmt.Sprintf("%.2f", l.temperature),
		"-n", "8192",
		"-p", fullPrompt,
		"--no-display-prompt",
	}

	cmd := exec.CommandContext(ctx, l.binaryPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("llama.cpp error: %w\nOutput: %s", err, string(out[:min(len(out), 500)]))
	}

	text := strings.TrimSpace(string(out))
	jsonStr := extractJSON(text)

	var proposal Proposal
	if err := json.Unmarshal([]byte(jsonStr), &proposal); err != nil {
		return nil, fmt.Errorf("parsing proposal JSON from llama.cpp: %w\nRaw: %s", err, text[:min(len(text), 500)])
	}

	if proposal.TargetFile == "" || proposal.NewContent == "" {
		return nil, fmt.Errorf("incomplete proposal from llama.cpp")
	}

	return &proposal, nil
}
